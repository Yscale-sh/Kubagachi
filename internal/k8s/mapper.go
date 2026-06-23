package k8s

import (
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/jakenesler/kubagachi/internal/state"
)

// MapPod converts a Kubernetes pod into a normalized PodView, including the
// derived health status and the deterministically assigned critter.
func MapPod(pod *corev1.Pod) state.PodView {
	pv := state.PodView{
		UID:       string(pod.UID),
		Name:      pod.Name,
		Namespace: pod.Namespace,
		NodeName:  pod.Spec.NodeName,
		IP:        pod.Status.PodIP,
		Phase:     string(pod.Status.Phase),
		Age:       humanizeAge(pod.CreationTimestamp.Time),
	}
	if !pod.CreationTimestamp.IsZero() {
		pv.AgeSeconds = int64(time.Since(pod.CreationTimestamp.Time).Seconds())
	}
	pv.CPUUsedMilli = -1
	pv.MemUsedBytes = -1
	if len(pod.OwnerReferences) > 0 {
		pv.Owner = pod.OwnerReferences[0].Name
	}
	for _, cs := range pod.Status.ContainerStatuses {
		pv.Containers = append(pv.Containers, mapContainer(cs))
		pv.Restarts += cs.RestartCount
	}

	pv.Status, pv.Reason = detectStatus(pod)
	pv.CritterState = pv.Status

	// Deterministic critter: keyed on the owner so every replica of a
	// Deployment/StatefulSet shares the same animal identity.
	key := pod.Namespace + "/" + pod.Name
	if pv.Owner != "" {
		key = pod.Namespace + "/" + pv.Owner
	}
	// Project mascots are reserved: yscale-family → Nori, cartogopher → the
	// gopher, everything else → the general pool (never Nori/Cartogopher).
	pv.Critter = assignCritter(pv.Namespace, pv.Owner, key)

	// Yscale workload overlay: a healthy Nori pod whose namespace/owner carries a
	// workload keyword (burst/gpu/edge/drain/scale) performs that activity.
	applyWorkloadAnimation(&pv)
	return pv
}

func mapContainer(cs corev1.ContainerStatus) state.ContainerView {
	cv := state.ContainerView{
		Name:         cs.Name,
		Image:        cs.Image,
		Ready:        cs.Ready,
		RestartCount: cs.RestartCount,
	}
	switch {
	case cs.State.Running != nil:
		cv.State = "running"
	case cs.State.Waiting != nil:
		cv.State = "waiting"
		cv.Reason = cs.State.Waiting.Reason
		cv.Message = singleLine(cs.State.Waiting.Message)
	case cs.State.Terminated != nil:
		cv.State = "terminated"
		cv.Reason = cs.State.Terminated.Reason
		cv.Message = singleLine(cs.State.Terminated.Message)
		cv.ExitCode = cs.State.Terminated.ExitCode
	default:
		cv.State = "unknown"
	}
	return cv
}

// detectStatus derives the normalized health status from the pod phase,
// deletion timestamp and container states, mirroring how kubectl computes the
// displayed pod status.
func detectStatus(pod *corev1.Pod) (status, reason string) {
	if pod.DeletionTimestamp != nil {
		return state.StatusTerminating, "Terminating"
	}

	phase := pod.Status.Phase
	reason = string(phase)
	if pod.Status.Reason != "" {
		reason = pod.Status.Reason
	}

	containers := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	containers = append(containers, pod.Status.ContainerStatuses...)
	for _, cs := range containers {
		if s, r, ok := containerSignal(cs); ok {
			return s, r
		}
	}

	switch phase {
	case corev1.PodRunning:
		return state.StatusRunning, "Running"
	case corev1.PodPending:
		return state.StatusPending, reason
	case corev1.PodSucceeded:
		return state.StatusCompleted, "Completed"
	case corev1.PodFailed:
		return state.StatusFailed, reason
	default:
		return state.StatusUnknown, "Unknown"
	}
}

// containerSignal reports an overriding health status when a container is in
// a notable waiting/terminated state (crash loops, image pull failures, OOM).
func containerSignal(cs corev1.ContainerStatus) (status, reason string, ok bool) {
	if w := cs.State.Waiting; w != nil {
		switch w.Reason {
		case "CrashLoopBackOff":
			return state.StatusCrashLoop, w.Reason, true
		case "ImagePullBackOff", "ErrImagePull", "ImagePullBackoff":
			return state.StatusImagePull, w.Reason, true
		case "CreateContainerError", "CreateContainerConfigError", "RunContainerError":
			return state.StatusFailed, w.Reason, true
		}
	}
	if t := cs.State.Terminated; t != nil {
		if t.Reason == "OOMKilled" {
			return state.StatusOOMKilled, t.Reason, true
		}
	}
	if lt := cs.LastTerminationState.Terminated; lt != nil && lt.Reason == "OOMKilled" {
		return state.StatusOOMKilled, "OOMKilled", true
	}
	return "", "", false
}

// MapNode converts a Kubernetes node into a normalized NodeView.
func MapNode(n *corev1.Node) state.NodeView {
	nv := state.NodeView{Name: n.Name}
	for _, c := range n.Status.Conditions {
		if c.Type == corev1.NodeReady {
			nv.Ready = c.Status == corev1.ConditionTrue
		}
	}
	cpu := n.Status.Allocatable.Cpu()
	mem := n.Status.Allocatable.Memory()
	nv.CPUText = cpu.String() + " cpu"
	nv.MemoryText = humanizeBytes(mem.Value())
	nv.CPUMilli = cpu.MilliValue()
	nv.MemBytes = mem.Value()
	nv.CPUUsedMilli = -1
	nv.MemUsedBytes = -1
	return nv
}

// MapEvents converts and sorts Kubernetes events newest-first, capping the
// feed so the TUI never holds an unbounded slice.
func MapEvents(events []*corev1.Event) []state.EventView {
	type stamped struct {
		view state.EventView
		when time.Time
	}
	tmp := make([]stamped, 0, len(events))
	for _, e := range events {
		when := eventTime(e)
		tmp = append(tmp, stamped{
			view: state.EventView{
				Time:    humanizeAge(when),
				Type:    e.Type,
				Reason:  e.Reason,
				Object:  e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name,
				Message: singleLine(e.Message),
			},
			when: when,
		})
	}
	sort.SliceStable(tmp, func(i, j int) bool { return tmp[i].when.After(tmp[j].when) })

	const maxEvents = 200
	out := make([]state.EventView, 0, len(tmp))
	for _, s := range tmp {
		out = append(out, s.view)
	}
	if len(out) > maxEvents {
		out = out[:maxEvents]
	}
	return out
}

func eventTime(e *corev1.Event) time.Time {
	if !e.LastTimestamp.IsZero() {
		return e.LastTimestamp.Time
	}
	if !e.EventTime.IsZero() {
		return e.EventTime.Time
	}
	return e.FirstTimestamp.Time
}

func humanizeAge(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return humanizeDuration(time.Since(t))
}

func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func humanizeBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%dB", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

func singleLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}
