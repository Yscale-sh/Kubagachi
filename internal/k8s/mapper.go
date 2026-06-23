package k8s

import (
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
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
				Time:      humanizeAge(when),
				Type:      e.Type,
				Reason:    e.Reason,
				Object:    e.InvolvedObject.Kind + "/" + e.InvolvedObject.Name,
				Message:   singleLine(e.Message),
				Namespace: eventNamespace(e),
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

// MapDeployment converts an apps/v1 Deployment into a normalized DeploymentView.
func MapDeployment(d *appsv1.Deployment) state.DeploymentView {
	dv := state.DeploymentView{
		Name:      d.Name,
		Namespace: d.Namespace,
		Ready:     d.Status.ReadyReplicas,
		Updated:   d.Status.UpdatedReplicas,
		Available: d.Status.AvailableReplicas,
		Age:       humanizeAge(d.CreationTimestamp.Time),
	}
	if d.Spec.Replicas != nil {
		dv.Replicas = *d.Spec.Replicas
	}
	if !d.CreationTimestamp.IsZero() {
		dv.AgeSeconds = int64(time.Since(d.CreationTimestamp.Time).Seconds())
	}
	if containers := d.Spec.Template.Spec.Containers; len(containers) > 0 {
		dv.Image = containers[0].Image
	}
	if d.Spec.Selector != nil {
		dv.Selector = renderSelector(d.Spec.Selector.MatchLabels)
	}
	return dv
}

// MapService converts a core/v1 Service into a normalized ServiceView.
func MapService(s *corev1.Service) state.ServiceView {
	sv := state.ServiceView{
		Name:      s.Name,
		Namespace: s.Namespace,
		Type:      string(s.Spec.Type),
		ClusterIP: s.Spec.ClusterIP,
		Selector:  renderSelector(s.Spec.Selector),
		Age:       humanizeAge(s.CreationTimestamp.Time),
	}
	if sv.Type == "" {
		sv.Type = string(corev1.ServiceTypeClusterIP)
	}
	// A headless service (clusterIP None) is its own logical kind in the UI.
	if s.Spec.ClusterIP == corev1.ClusterIPNone {
		sv.Type = "Headless"
	}
	if !s.CreationTimestamp.IsZero() {
		sv.AgeSeconds = int64(time.Since(s.CreationTimestamp.Time).Seconds())
	}
	sv.ExternalIP = serviceExternalIP(s)
	for _, p := range s.Spec.Ports {
		proto := string(p.Protocol)
		if proto == "" {
			proto = string(corev1.ProtocolTCP)
		}
		sv.Ports = append(sv.Ports, state.ServicePortView{
			Name:       p.Name,
			Port:       p.Port,
			TargetPort: p.TargetPort.IntVal,
			NodePort:   p.NodePort,
			Protocol:   proto,
		})
	}
	return sv
}

// serviceExternalIP resolves the first externally reachable address: an
// explicit spec.externalIP, an ExternalName, or a load-balancer ingress entry.
func serviceExternalIP(s *corev1.Service) string {
	if len(s.Spec.ExternalIPs) > 0 {
		return s.Spec.ExternalIPs[0]
	}
	if s.Spec.Type == corev1.ServiceTypeExternalName && s.Spec.ExternalName != "" {
		return s.Spec.ExternalName
	}
	for _, ing := range s.Status.LoadBalancer.Ingress {
		if ing.IP != "" {
			return ing.IP
		}
		if ing.Hostname != "" {
			return ing.Hostname
		}
	}
	return ""
}

// MapConfigMap converts a core/v1 ConfigMap into a normalized ConfigMapView.
func MapConfigMap(c *corev1.ConfigMap) state.ConfigMapView {
	cv := state.ConfigMapView{
		Name:      c.Name,
		Namespace: c.Namespace,
		Age:       humanizeAge(c.CreationTimestamp.Time),
	}
	if !c.CreationTimestamp.IsZero() {
		cv.AgeSeconds = int64(time.Since(c.CreationTimestamp.Time).Seconds())
	}
	keys := make([]string, 0, len(c.Data)+len(c.BinaryData))
	for k, v := range c.Data {
		keys = append(keys, k)
		cv.DataBytes += len(v)
	}
	for k, v := range c.BinaryData {
		keys = append(keys, k)
		cv.DataBytes += len(v)
	}
	sort.Strings(keys)
	cv.Keys = keys
	return cv
}

// renderSelector renders a label map as a stable "k=v,k=v" string with sorted
// keys, so the UI sees a deterministic value across snapshots.
func renderSelector(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+m[k])
	}
	return strings.Join(parts, ",")
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

// eventNamespace resolves the namespace the event belongs to: the involved
// object's namespace is canonical; fall back to the Event resource's own
// namespace. Both are empty for cluster-scoped objects (e.g. Node events).
func eventNamespace(e *corev1.Event) string {
	if ns := e.InvolvedObject.Namespace; ns != "" {
		return ns
	}
	return e.Namespace
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
