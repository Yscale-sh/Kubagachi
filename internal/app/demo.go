package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jakenesler/kubagachi/internal/critters"
	"github.com/jakenesler/kubagachi/internal/state"
	"github.com/jakenesler/kubagachi/internal/tui"
)

// demoSource streams a hand-crafted fake cluster so kubagachi can be run
// and explored without any Kubernetes access.
type demoSource struct {
	cfg Config
}

func (s demoSource) Label() string { return "demo" }

func (s demoSource) Actions() tui.Actions { return demoActions{} }

func (s demoSource) Stream(ctx context.Context) (<-chan state.ClusterState, error) {
	out := make(chan state.ClusterState, 4)
	go s.run(ctx, out)
	return out, nil
}

// run emits an initial snapshot and then keeps the demo feeling alive by
// nudging restart counts and prepending fresh events on a timer.
func (s demoSource) run(ctx context.Context, out chan<- state.ClusterState) {
	defer close(out)

	snap := s.build()
	send := func() bool {
		select {
		case out <- snap:
			return true
		case <-ctx.Done():
			return false
		}
	}
	if !send() {
		return
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	iteration := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			iteration++
			snap = s.build()
			// Crash-looping critters keep racking up restarts over time.
			for i := range snap.Pods {
				if snap.Pods[i].Status == state.StatusCrashLoop {
					snap.Pods[i].Restarts += int32(iteration)
				}
			}
			// Breathe the CPU/MEM bars with a small per-tick wave so the
			// habitat feels live (clamped to allocatable).
			for i := range snap.Nodes {
				n := &snap.Nodes[i]
				wave := int64((iteration*7+i*53)%21) - 10 // -10..10 %
				n.CPUUsedMilli = clampUsage(n.CPUUsedMilli+n.CPUMilli*wave/100, n.CPUMilli)
				n.MemUsedBytes = clampUsage(n.MemUsedBytes+n.MemBytes*wave/200, n.MemBytes)
			}
			snap.Events = append([]state.EventView{{
				Time:    "0s",
				Type:    "Normal",
				Reason:  "Heartbeat",
				Object:  "Cluster/demo",
				Message: fmt.Sprintf("demo tick %d — critters are alive", iteration),
				// Cluster-scoped: no namespace.
			}}, snap.Events...)
			snap.Rebuild()
			if !send() {
				return
			}
		}
	}
}

// build constructs the full fake ClusterState.
func (s demoSource) build() state.ClusterState {
	allNS := s.cfg.AllNamespaces || (s.cfg.Namespace == "" && !s.cfg.AllNamespaces)
	cs := state.ClusterState{
		ClusterName:      "demo-cluster",
		Namespace:        s.cfg.Namespace,
		AllNamespaces:    allNS,
		MetricsInstalled: true,
		Nodes: []state.NodeView{
			{Name: "critter-node-a", Ready: true, CPUText: "4 cpu", MemoryText: "16.0GiB", CPUMilli: 4000, MemBytes: 16 << 30},
			{Name: "critter-node-b", Ready: true, CPUText: "8 cpu", MemoryText: "32.0GiB", CPUMilli: 8000, MemBytes: 32 << 30},
			{Name: "critter-node-c", Ready: false, CPUText: "4 cpu", MemoryText: "16.0GiB", CPUMilli: 4000, MemBytes: 16 << 30},
		},
	}

	type spec struct {
		ns, name, node, owner, status string
		restarts                      int32
		ageMin                        int
	}
	specs := []spec{
		{"default", "web-frontend-7d9c-aa", "critter-node-a", "web-frontend", state.StatusRunning, 0, 320},
		{"default", "web-frontend-7d9c-bb", "critter-node-b", "web-frontend", state.StatusRunning, 0, 320},
		{"default", "web-frontend-7d9c-cc", "critter-node-c", "web-frontend", state.StatusRunning, 1, 90},
		{"default", "api-gateway-58f4-aa", "critter-node-a", "api-gateway", state.StatusRunning, 0, 1500},
		{"default", "api-gateway-58f4-bb", "critter-node-b", "api-gateway", state.StatusCrashLoop, 7, 45},
		{"default", "payments-6b2d-aa", "critter-node-b", "payments", state.StatusImagePull, 0, 12},
		{"default", "cache-redis-0", "critter-node-a", "cache-redis", state.StatusRunning, 0, 4300},
		{"default", "cache-redis-1", "critter-node-c", "cache-redis", state.StatusPending, 0, 3},
		{"default", "batch-report-29-x", "critter-node-a", "batch-report", state.StatusCompleted, 0, 60},
		{"default", "batch-report-30-x", "critter-node-b", "batch-report", state.StatusCompleted, 0, 30},
		{"default", "migration-runner-zz", "critter-node-b", "", state.StatusFailed, 2, 25},
		{"kube-system", "coredns-5d78-aa", "critter-node-a", "coredns", state.StatusRunning, 0, 5000},
		{"kube-system", "coredns-5d78-bb", "critter-node-b", "coredns", state.StatusRunning, 0, 5000},
		{"kube-system", "kube-proxy-aa", "critter-node-a", "kube-proxy", state.StatusRunning, 0, 5000},
		{"kube-system", "metrics-server-aa", "critter-node-c", "metrics-server", state.StatusBackOff, 4, 18},
		{"monitoring", "prometheus-0", "critter-node-b", "prometheus", state.StatusRunning, 1, 2600},
		{"monitoring", "grafana-77c9-aa", "critter-node-a", "grafana", state.StatusRunning, 0, 2600},
		{"monitoring", "loki-ingester-3", "critter-node-c", "loki-ingester", state.StatusOOMKilled, 3, 70},
		{"monitoring", "node-exporter-zz", "", "node-exporter", state.StatusUnknown, 0, 40},
		{"web", "shop-checkout-9f-aa", "critter-node-a", "shop-checkout", state.StatusTerminating, 0, 200},
	}

	for _, sp := range specs {
		if !allNS && s.cfg.Namespace != "" && sp.ns != s.cfg.Namespace {
			continue
		}
		cs.Pods = append(cs.Pods, demoPod(sp.ns, sp.name, sp.node, sp.owner, sp.status, sp.restarts, sp.ageMin))
	}

	// Roll up per-pod usage into node usage (+ a little kubelet/system
	// overhead) so the habitat's CPU/MEM bars are driven by real-shaped data.
	usageByNode := map[string]struct{ cpu, mem int64 }{}
	for _, p := range cs.Pods {
		u := usageByNode[p.NodeName]
		u.cpu += p.CPUUsedMilli
		u.mem += p.MemUsedBytes
		usageByNode[p.NodeName] = u
	}
	for i := range cs.Nodes {
		n := &cs.Nodes[i]
		u := usageByNode[n.Name]
		// System overhead scales with capacity.
		n.CPUUsedMilli = u.cpu + n.CPUMilli/8
		n.MemUsedBytes = u.mem + n.MemBytes/6
		if n.CPUUsedMilli > n.CPUMilli {
			n.CPUUsedMilli = n.CPUMilli
		}
		if n.MemUsedBytes > n.MemBytes {
			n.MemUsedBytes = n.MemBytes
		}
	}

	cs.Events = demoEvents()
	cs.Flux = demoFlux()
	cs.FluxInstalled = true
	cs.Rebuild()
	return cs
}

// clampUsage keeps a usage value within [0, max].
func clampUsage(v, max int64) int64 {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

// demoUsage returns plausible CPU (millicores) and memory (bytes) usage for a
// pod given its status and a stable seed.
func demoUsage(status, seed string) (int64, int64) {
	h := int64(0)
	for _, c := range seed {
		h = h*31 + int64(c)
	}
	if h < 0 {
		h = -h
	}
	switch status {
	case state.StatusRunning:
		return 60 + h%240, (96 + h%320) << 20 // 60-300m, 96-416Mi
	case state.StatusCrashLoop, state.StatusOOMKilled, state.StatusFailed:
		return 5 + h%30, (24 + h%64) << 20
	case state.StatusPending, state.StatusImagePull, state.StatusBackOff, state.StatusTerminating:
		return h % 15, (8 + h%24) << 20
	case state.StatusCompleted:
		return 0, 0
	default:
		return h % 20, (16 + h%48) << 20
	}
}

// demoFlux fakes a small GitOps estate so the flux view is explorable offline.
func demoFlux() []state.FluxView {
	return []state.FluxView{
		{Kind: "GitRepository", Name: "platform", Namespace: "flux-system", Ready: "True",
			Revision: "main@8f3d92a1", Message: "stored artifact for revision", Age: "21d"},
		{Kind: "Kustomization", Name: "flux-system", Namespace: "flux-system", Ready: "True",
			Revision: "main@8f3d92a1", Source: "GitRepository/platform",
			Message: "Applied revision main@8f3d92a1", Age: "21d"},
		{Kind: "Kustomization", Name: "apps", Namespace: "flux-system", Ready: "False",
			Revision: "main@77ac01be", Source: "GitRepository/platform",
			Message: "kustomize build failed: Deployment/web-frontend image not found", Age: "21d"},
		{Kind: "Kustomization", Name: "monitoring", Namespace: "flux-system", Ready: "True",
			Suspended: true, Revision: "main@5d11c0fe", Source: "GitRepository/platform",
			Message: "reconciliation suspended", Age: "14d"},
		{Kind: "HelmRepository", Name: "bitnami", Namespace: "flux-system", Ready: "True",
			Revision: "sha256:1b4c11e9", Message: "stored artifact", Age: "21d"},
		{Kind: "HelmRelease", Name: "ingress-nginx", Namespace: "ingress", Ready: "True",
			Revision: "4.10.1", Source: "HelmRepository/bitnami",
			Message: "Helm install succeeded", Age: "21d"},
		{Kind: "HelmRelease", Name: "grafana", Namespace: "monitoring", Ready: "False",
			Revision: "7.3.0", Source: "HelmRepository/bitnami",
			Message: "Helm upgrade failed: timed out waiting for the condition", Age: "9d"},
	}
}

func demoPod(ns, name, node, owner, status string, restarts int32, ageMin int) state.PodView {
	key := ns + "/" + name
	if owner != "" {
		key = ns + "/" + owner
	}
	p := state.PodView{
		UID:          "demo-" + ns + "-" + name,
		Name:         name,
		Namespace:    ns,
		NodeName:     node,
		IP:           fmt.Sprintf("10.244.%d.%d", len(ns)%6+1, (len(name)*7)%250),
		Owner:        owner,
		Status:       status,
		CritterState: status,
		Phase:        demoPhase(status),
		Reason:       demoReason(status),
		Restarts:     restarts,
		Age:          demoAge(ageMin),
		AgeSeconds:   int64(ageMin) * 60,
		Critter:      critters.Assign(key),
	}
	p.CPUUsedMilli, p.MemUsedBytes = demoUsage(status, ns+"/"+name)
	p.Containers = []state.ContainerView{demoContainer(name, status, restarts)}
	return p
}

func demoContainer(name, status string, restarts int32) state.ContainerView {
	c := state.ContainerView{Name: name, RestartCount: restarts}
	switch status {
	case state.StatusRunning:
		c.Ready, c.State = true, "running"
	case state.StatusCompleted:
		c.State, c.Reason, c.ExitCode = "terminated", "Completed", 0
	case state.StatusFailed:
		c.State, c.Reason, c.ExitCode = "terminated", "Error", 1
	case state.StatusOOMKilled:
		c.State, c.Reason, c.ExitCode = "terminated", "OOMKilled", 137
	case state.StatusCrashLoop:
		c.State, c.Reason = "waiting", "CrashLoopBackOff"
	case state.StatusImagePull:
		c.State, c.Reason = "waiting", "ImagePullBackOff"
	case state.StatusBackOff:
		c.State, c.Reason = "waiting", "BackOff"
	case state.StatusPending:
		c.State, c.Reason = "waiting", "ContainerCreating"
	case state.StatusTerminating:
		c.State, c.Reason = "running", "Terminating"
	default:
		c.State, c.Reason = "unknown", "Unknown"
	}
	return c
}

func demoPhase(status string) string {
	switch status {
	case state.StatusRunning, state.StatusTerminating:
		return "Running"
	case state.StatusPending, state.StatusImagePull, state.StatusBackOff:
		return "Pending"
	case state.StatusCompleted:
		return "Succeeded"
	case state.StatusFailed, state.StatusOOMKilled, state.StatusCrashLoop:
		return "Failed"
	default:
		return "Unknown"
	}
}

func demoReason(status string) string {
	switch status {
	case state.StatusRunning:
		return "Running"
	case state.StatusPending:
		return "ContainerCreating"
	case state.StatusCrashLoop:
		return "CrashLoopBackOff"
	case state.StatusImagePull:
		return "ImagePullBackOff"
	case state.StatusBackOff:
		return "BackOff"
	case state.StatusOOMKilled:
		return "OOMKilled"
	case state.StatusCompleted:
		return "Completed"
	case state.StatusFailed:
		return "Error"
	case state.StatusTerminating:
		return "Terminating"
	default:
		return "Unknown"
	}
}

func demoAge(min int) string {
	switch {
	case min < 60:
		return fmt.Sprintf("%dm", min)
	case min < 60*24:
		return fmt.Sprintf("%dh", min/60)
	default:
		return fmt.Sprintf("%dd", min/(60*24))
	}
}

func demoEvents() []state.EventView {
	return []state.EventView{
		{Time: "12s", Type: "Warning", Reason: "BackOff", Object: "Pod/api-gateway-58f4-bb", Namespace: "default", Message: "Back-off restarting failed container"},
		{Time: "30s", Type: "Warning", Reason: "Failed", Object: "Pod/payments-6b2d-aa", Namespace: "default", Message: "Failed to pull image: not found"},
		{Time: "1m", Type: "Normal", Reason: "Scheduled", Object: "Pod/cache-redis-1", Namespace: "default", Message: "Successfully assigned default/cache-redis-1"},
		{Time: "2m", Type: "Warning", Reason: "OOMKilling", Object: "Pod/metrics-server-aa", Namespace: "kube-system", Message: "Memory cgroup out of memory: killed process"},
		{Time: "3m", Type: "Normal", Reason: "Pulled", Object: "Pod/coredns-5d78-aa", Namespace: "kube-system", Message: "Container image already present on machine"},
		{Time: "4m", Type: "Warning", Reason: "NodeNotReady", Object: "Node/critter-node-c", Message: "Node critter-node-c status is now NotReady"},
		{Time: "6m", Type: "Normal", Reason: "Completed", Object: "Pod/batch-report-30-x", Namespace: "default", Message: "Job completed successfully"},
		{Time: "9m", Type: "Normal", Reason: "Killing", Object: "Pod/api-gateway-58f4-aa", Namespace: "default", Message: "Stopping container api-gateway"},
	}
}
