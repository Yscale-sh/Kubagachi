package app

import (
	"context"
	"fmt"
	"time"

	"github.com/yscale-sh/kubagachi/internal/critters"
	"github.com/yscale-sh/kubagachi/internal/state"
	"github.com/yscale-sh/kubagachi/internal/tui"
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

	nsAllowed := func(ns string) bool {
		return allNS || s.cfg.Namespace == "" || ns == s.cfg.Namespace
	}
	for _, n := range demoNamespaces() {
		if nsAllowed(n.Name) {
			cs.Namespaces = append(cs.Namespaces, n)
		}
	}
	for _, d := range demoDeployments() {
		if nsAllowed(d.Namespace) {
			cs.Deployments = append(cs.Deployments, d)
		}
	}
	for _, s := range demoStatefulSets() {
		if nsAllowed(s.Namespace) {
			cs.StatefulSets = append(cs.StatefulSets, s)
		}
	}
	for _, d := range demoDaemonSets() {
		if nsAllowed(d.Namespace) {
			cs.DaemonSets = append(cs.DaemonSets, d)
		}
	}
	for _, r := range demoReplicaSets() {
		if nsAllowed(r.Namespace) {
			cs.ReplicaSets = append(cs.ReplicaSets, r)
		}
	}
	for _, j := range demoJobs() {
		if nsAllowed(j.Namespace) {
			cs.Jobs = append(cs.Jobs, j)
		}
	}
	for _, c := range demoCronJobs() {
		if nsAllowed(c.Namespace) {
			cs.CronJobs = append(cs.CronJobs, c)
		}
	}
	for _, sv := range demoServices() {
		if nsAllowed(sv.Namespace) {
			cs.Services = append(cs.Services, sv)
		}
	}
	for _, i := range demoIngresses() {
		if nsAllowed(i.Namespace) {
			cs.Ingresses = append(cs.Ingresses, i)
		}
	}
	for _, e := range demoEndpoints() {
		if nsAllowed(e.Namespace) {
			cs.Endpoints = append(cs.Endpoints, e)
		}
	}
	for _, n := range demoNetworkPolicies() {
		if nsAllowed(n.Namespace) {
			cs.NetworkPolicies = append(cs.NetworkPolicies, n)
		}
	}
	for _, cm := range demoConfigMaps() {
		if nsAllowed(cm.Namespace) {
			cs.ConfigMaps = append(cs.ConfigMaps, cm)
		}
	}
	for _, s := range demoSecrets() {
		if nsAllowed(s.Namespace) {
			cs.Secrets = append(cs.Secrets, s)
		}
	}
	for _, r := range demoResourceQuotas() {
		if nsAllowed(r.Namespace) {
			cs.ResourceQuotas = append(cs.ResourceQuotas, r)
		}
	}
	for _, l := range demoLimitRanges() {
		if nsAllowed(l.Namespace) {
			cs.LimitRanges = append(cs.LimitRanges, l)
		}
	}
	for _, h := range demoHorizontalPodAutoscalers() {
		if nsAllowed(h.Namespace) {
			cs.HorizontalPodAutoscalers = append(cs.HorizontalPodAutoscalers, h)
		}
	}
	for _, p := range demoPodDisruptionBudgets() {
		if nsAllowed(p.Namespace) {
			cs.PodDisruptionBudgets = append(cs.PodDisruptionBudgets, p)
		}
	}
	for _, s := range demoServiceAccounts() {
		if nsAllowed(s.Namespace) {
			cs.ServiceAccounts = append(cs.ServiceAccounts, s)
		}
	}
	for _, r := range demoRoles() {
		if nsAllowed(r.Namespace) {
			cs.Roles = append(cs.Roles, r)
		}
	}
	cs.ClusterRoles = demoClusterRoles()
	for _, r := range demoRoleBindings() {
		if nsAllowed(r.Namespace) {
			cs.RoleBindings = append(cs.RoleBindings, r)
		}
	}
	cs.ClusterRoleBindings = demoClusterRoleBindings()
	cs.CustomResourceDefinitions = demoCustomResourceDefinitions()
	for _, p := range demoPersistentVolumeClaims() {
		if nsAllowed(p.Namespace) {
			cs.PersistentVolumeClaims = append(cs.PersistentVolumeClaims, p)
		}
	}
	cs.PersistentVolumes = demoPersistentVolumes()
	cs.StorageClasses = demoStorageClasses()
	for _, h := range demoHelmReleases() {
		if nsAllowed(h.Namespace) {
			cs.HelmReleases = append(cs.HelmReleases, h)
		}
	}

	cs.Rebuild()
	return cs
}

// demoDeployments mirrors the demo pod owners so the Deployments tab reflects
// the same workloads. api-gateway is degraded (2/3) since one replica crashloops.
func demoDeployments() []state.DeploymentView {
	mk := func(ns, name string, replicas, ready int32, image, selector string, ageMin int) state.DeploymentView {
		return state.DeploymentView{
			Name: name, Namespace: ns,
			Replicas: replicas, Ready: ready, Updated: replicas, Available: ready,
			Image: image, Selector: selector,
			Age: demoAge(ageMin), AgeSeconds: int64(ageMin) * 60,
		}
	}
	return []state.DeploymentView{
		mk("default", "web-frontend", 3, 3, "ghcr.io/acme/web-frontend:1.8.2", "app=web-frontend", 320),
		mk("default", "api-gateway", 3, 2, "ghcr.io/acme/api-gateway:2.1.0", "app=api-gateway", 1500),
		mk("default", "payments", 2, 1, "ghcr.io/acme/payments:0.9.4", "app=payments", 12),
		mk("default", "cache-redis", 2, 1, "redis:7.2-alpine", "app=cache-redis", 4300),
		mk("default", "batch-report", 0, 0, "ghcr.io/acme/batch-report:3.0.1", "app=batch-report", 60),
		mk("kube-system", "coredns", 2, 2, "registry.k8s.io/coredns/coredns:v1.11.1", "k8s-app=kube-dns", 5000),
		mk("kube-system", "metrics-server", 1, 0, "registry.k8s.io/metrics-server/metrics-server:v0.7.1", "k8s-app=metrics-server", 18),
		mk("monitoring", "grafana", 1, 1, "grafana/grafana:10.4.2", "app=grafana", 2600),
		mk("monitoring", "prometheus", 1, 1, "prom/prometheus:v2.51.0", "app=prometheus", 2600),
	}
}

func demoStatefulSets() []state.StatefulSetView {
	return []state.StatefulSetView{
		{Name: "cache-redis", Namespace: "default", Replicas: 2, ReadyReplicas: 1, ServiceName: "cache-redis",
			Image: "redis:7.2-alpine", Age: demoAge(4300), AgeSeconds: 4300 * 60},
		{Name: "prometheus", Namespace: "monitoring", Replicas: 1, ReadyReplicas: 1, ServiceName: "prometheus",
			Image: "prom/prometheus:v2.51.0", Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoDaemonSets() []state.DaemonSetView {
	return []state.DaemonSetView{
		{Name: "kube-proxy", Namespace: "kube-system", DesiredNumberScheduled: 3, NumberReady: 3, NumberAvailable: 3,
			Image: "registry.k8s.io/kube-proxy:v1.30.0", Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "node-exporter", Namespace: "monitoring", DesiredNumberScheduled: 3, NumberReady: 2, NumberAvailable: 2,
			Image: "quay.io/prometheus/node-exporter:v1.8.0", Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoReplicaSets() []state.ReplicaSetView {
	return []state.ReplicaSetView{
		{Name: "web-frontend-7d9c", Namespace: "default", Replicas: 3, ReadyReplicas: 3, OwnerKind: "Deployment", OwnerName: "web-frontend",
			Image: "ghcr.io/acme/web-frontend:1.8.2", Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "api-gateway-58f4", Namespace: "default", Replicas: 3, ReadyReplicas: 2, OwnerKind: "Deployment", OwnerName: "api-gateway",
			Image: "ghcr.io/acme/api-gateway:2.1.0", Age: demoAge(1500), AgeSeconds: 1500 * 60},
	}
}

func demoJobs() []state.JobView {
	return []state.JobView{
		{Name: "batch-report-30", Namespace: "default", Completions: 1, Succeeded: 1, Status: "completed",
			Image: "ghcr.io/acme/batch-report:3.0.1", DurationSec: 180, Age: demoAge(30), AgeSeconds: 30 * 60},
		{Name: "migration-runner", Namespace: "default", Completions: 1, Failed: 1, Status: "failed",
			Image: "ghcr.io/acme/migrations:2.4.0", DurationSec: 75, Age: demoAge(25), AgeSeconds: 25 * 60},
	}
}

func demoCronJobs() []state.CronJobView {
	return []state.CronJobView{
		{Name: "batch-report", Namespace: "default", Schedule: "*/30 * * * *", ActiveJobs: 0, Status: "active",
			Image: "ghcr.io/acme/batch-report:3.0.1", HasLastSchedule: true, LastScheduleAgeSec: 30 * 60,
			Age: demoAge(1440), AgeSeconds: 1440 * 60},
		{Name: "db-backup", Namespace: "monitoring", Schedule: "0 3 * * *", Suspend: true, Status: "suspended",
			Image: "ghcr.io/acme/db-backup:1.2.0", HasLastSchedule: true, LastScheduleAgeSec: 9 * 3600,
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

// demoServices fakes a handful of services across namespaces, including a
// LoadBalancer, a headless service and a UDP DNS service.
func demoServices() []state.ServiceView {
	return []state.ServiceView{
		{Name: "web-frontend", Namespace: "default", Type: "ClusterIP", ClusterIP: "10.96.10.20",
			Selector: "app=web-frontend", Age: demoAge(320), AgeSeconds: 320 * 60,
			Ports: []state.ServicePortView{{Name: "http", Port: 80, TargetPort: 8080, Protocol: "TCP"}}},
		{Name: "api-gateway", Namespace: "default", Type: "LoadBalancer", ClusterIP: "10.96.10.40", ExternalIP: "203.0.113.17",
			Selector: "app=api-gateway", Age: demoAge(1500), AgeSeconds: 1500 * 60,
			Ports: []state.ServicePortView{{Name: "https", Port: 443, TargetPort: 8443, NodePort: 31443, Protocol: "TCP"}}},
		{Name: "cache-redis", Namespace: "default", Type: "Headless", ClusterIP: "None",
			Selector: "app=cache-redis", Age: demoAge(4300), AgeSeconds: 4300 * 60,
			Ports: []state.ServicePortView{{Name: "redis", Port: 6379, TargetPort: 6379, Protocol: "TCP"}}},
		{Name: "kube-dns", Namespace: "kube-system", Type: "ClusterIP", ClusterIP: "10.96.0.10",
			Selector: "k8s-app=kube-dns", Age: demoAge(5000), AgeSeconds: 5000 * 60,
			Ports: []state.ServicePortView{
				{Name: "dns", Port: 53, TargetPort: 53, Protocol: "UDP"},
				{Name: "dns-tcp", Port: 53, TargetPort: 53, Protocol: "TCP"},
			}},
		{Name: "grafana", Namespace: "monitoring", Type: "ClusterIP", ClusterIP: "10.96.20.30",
			Selector: "app=grafana", Age: demoAge(2600), AgeSeconds: 2600 * 60,
			Ports: []state.ServicePortView{{Name: "http", Port: 3000, TargetPort: 3000, Protocol: "TCP"}}},
		{Name: "prometheus", Namespace: "monitoring", Type: "NodePort", ClusterIP: "10.96.20.31",
			Selector: "app=prometheus", Age: demoAge(2600), AgeSeconds: 2600 * 60,
			Ports: []state.ServicePortView{{Name: "web", Port: 9090, TargetPort: 9090, NodePort: 30090, Protocol: "TCP"}}},
	}
}

func demoIngresses() []state.IngressView {
	return []state.IngressView{
		{Name: "web-frontend", Namespace: "default", ClassName: "nginx", Hosts: []string{"app.demo.local"},
			Rules: []state.IngressRuleView{{Host: "app.demo.local", Path: "/", ServiceName: "web-frontend", ServicePort: 80}},
			TLS:   true, Address: "203.0.113.20", Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "grafana", Namespace: "monitoring", ClassName: "nginx", Hosts: []string{"grafana.demo.local"},
			Rules: []state.IngressRuleView{{Host: "grafana.demo.local", Path: "/", ServiceName: "grafana", ServicePort: 3000}},
			TLS:   false, Address: "203.0.113.21", Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoEndpoints() []state.EndpointView {
	return []state.EndpointView{
		{Name: "web-frontend", Namespace: "default", TargetService: "web-frontend",
			Subsets: []state.EndpointSubsetView{{Addresses: []string{"10.244.1.20", "10.244.2.21", "10.244.3.22"}, Ports: []int{8080}}},
			Age:     demoAge(320), AgeSeconds: 320 * 60},
		{Name: "api-gateway", Namespace: "default", TargetService: "api-gateway",
			Subsets: []state.EndpointSubsetView{{Addresses: []string{"10.244.1.30", "10.244.2.31"}, Ports: []int{8443}}},
			Age:     demoAge(1500), AgeSeconds: 1500 * 60},
		{Name: "kube-dns", Namespace: "kube-system", TargetService: "kube-dns",
			Subsets: []state.EndpointSubsetView{{Addresses: []string{"10.244.1.10", "10.244.2.10"}, Ports: []int{53}}},
			Age:     demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "grafana", Namespace: "monitoring", TargetService: "grafana",
			Subsets: []state.EndpointSubsetView{{Addresses: []string{"10.244.1.70"}, Ports: []int{3000}}},
			Age:     demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoNetworkPolicies() []state.NetworkPolicyView {
	return []state.NetworkPolicyView{
		{Name: "default-deny", Namespace: "default", PodSelector: map[string]string{},
			PolicyTypes: []string{"Ingress", "Egress"}, IngressRules: 0, EgressRules: 0,
			Age: demoAge(720), AgeSeconds: 720 * 60},
		{Name: "allow-frontend-to-api", Namespace: "default", PodSelector: map[string]string{"app": "api-gateway"},
			PolicyTypes: []string{"Ingress"}, IngressRules: 1, EgressRules: 0,
			Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "monitoring-scrape", Namespace: "monitoring", PodSelector: map[string]string{"app": "prometheus"},
			PolicyTypes: []string{"Ingress", "Egress"}, IngressRules: 2, EgressRules: 1,
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

// demoConfigMaps fakes a few config maps so the ConfigMaps tab is explorable.
func demoConfigMaps() []state.ConfigMapView {
	return []state.ConfigMapView{
		{Name: "app-config", Namespace: "default", Keys: []string{"app.yaml", "feature-flags.json", "log-level"},
			DataBytes: 2148, Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "nginx-conf", Namespace: "default", Keys: []string{"default.conf", "nginx.conf"},
			DataBytes: 1536, Age: demoAge(1500), AgeSeconds: 1500 * 60},
		{Name: "coredns", Namespace: "kube-system", Keys: []string{"Corefile"},
			DataBytes: 612, Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "kube-proxy", Namespace: "kube-system", Keys: []string{"config.conf", "kubeconfig.conf"},
			DataBytes: 1890, Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "grafana-dashboards", Namespace: "monitoring", Keys: []string{"cluster.json", "nodes.json", "pods.json"},
			DataBytes: 40960, Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoSecrets() []state.SecretView {
	return []state.SecretView{
		{Name: "web-tls", Namespace: "default", Type: "kubernetes.io/tls", Keys: []string{"tls.crt", "tls.key"},
			DataBytes: 4096, Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "registry-creds", Namespace: "default", Type: "kubernetes.io/dockerconfigjson", Keys: []string{".dockerconfigjson"},
			DataBytes: 1220, Age: demoAge(1500), AgeSeconds: 1500 * 60},
		{Name: "grafana-admin", Namespace: "monitoring", Type: "Opaque", Keys: []string{"admin-user", "admin-password"},
			DataBytes: 64, Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoResourceQuotas() []state.ResourceQuotaView {
	return []state.ResourceQuotaView{
		{Name: "default-quota", Namespace: "default",
			Hard: map[string]string{"pods": "80", "requests.cpu": "20", "requests.memory": "80Gi"},
			Used: map[string]string{"pods": "18", "requests.cpu": "8", "requests.memory": "24Gi"},
			Age:  demoAge(1440), AgeSeconds: 1440 * 60},
		{Name: "system-quota", Namespace: "kube-system",
			Hard: map[string]string{"pods": "60", "requests.cpu": "16", "requests.memory": "48Gi"},
			Used: map[string]string{"pods": "10", "requests.cpu": "5", "requests.memory": "14Gi"},
			Age:  demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "monitoring-quota", Namespace: "monitoring",
			Hard: map[string]string{"pods": "40", "requests.cpu": "12", "requests.memory": "64Gi"},
			Used: map[string]string{"pods": "7", "requests.cpu": "4", "requests.memory": "22Gi"},
			Age:  demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoLimitRanges() []state.LimitRangeView {
	return []state.LimitRangeView{
		{Name: "default-limits", Namespace: "default",
			Limits: []state.LimitRangeItemView{{
				Type:           "Container",
				DefaultRequest: map[string]string{"cpu": "100m", "memory": "128Mi"},
				DefaultLimit:   map[string]string{"cpu": "1", "memory": "1Gi"},
			}},
			Age: demoAge(1440), AgeSeconds: 1440 * 60},
		{Name: "monitoring-limits", Namespace: "monitoring",
			Limits: []state.LimitRangeItemView{{
				Type:           "Container",
				DefaultRequest: map[string]string{"cpu": "250m", "memory": "256Mi"},
				DefaultLimit:   map[string]string{"cpu": "2", "memory": "2Gi"},
			}},
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoHorizontalPodAutoscalers() []state.HorizontalPodAutoscalerView {
	return []state.HorizontalPodAutoscalerView{
		{Name: "api-gateway", Namespace: "default", TargetKind: "Deployment", TargetName: "api-gateway",
			MinReplicas: 2, MaxReplicas: 8, CurrentReplicas: 3,
			TargetCPUPercent: 70, CurrentCPUPercent: 62, HasTargetCPUPercent: true, HasCurrentCPUPercent: true,
			Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "web-frontend", Namespace: "default", TargetKind: "Deployment", TargetName: "web-frontend",
			MinReplicas: 3, MaxReplicas: 10, CurrentReplicas: 3,
			TargetCPUPercent: 65, CurrentCPUPercent: 41, HasTargetCPUPercent: true, HasCurrentCPUPercent: true,
			Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "prometheus", Namespace: "monitoring", TargetKind: "StatefulSet", TargetName: "prometheus",
			MinReplicas: 1, MaxReplicas: 3, CurrentReplicas: 1,
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoPodDisruptionBudgets() []state.PodDisruptionBudgetView {
	return []state.PodDisruptionBudgetView{
		{Name: "api-gateway", Namespace: "default", MinAvailable: "2",
			CurrentHealthy: 2, DesiredHealthy: 2, ExpectedPods: 3, Selector: map[string]string{"app": "api-gateway"},
			Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "web-frontend", Namespace: "default", MaxUnavailable: "25%",
			CurrentHealthy: 3, DesiredHealthy: 2, ExpectedPods: 3, Selector: map[string]string{"app": "web-frontend"},
			Age: demoAge(320), AgeSeconds: 320 * 60},
		{Name: "prometheus", Namespace: "monitoring", MinAvailable: "1",
			CurrentHealthy: 1, DesiredHealthy: 1, ExpectedPods: 1, Selector: map[string]string{"app": "prometheus"},
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoNamespaces() []state.NamespaceView {
	return []state.NamespaceView{
		{Name: "default", Phase: "Active", Labels: map[string]string{"kubernetes.io/metadata.name": "default"},
			Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "kube-system", Phase: "Active", Labels: map[string]string{"kubernetes.io/metadata.name": "kube-system"},
			Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "monitoring", Phase: "Active", Labels: map[string]string{"team": "platform"},
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
		{Name: "web", Phase: "Terminating", Labels: map[string]string{"team": "frontend"},
			Age: demoAge(2100), AgeSeconds: 2100 * 60},
		{Name: "flux-system", Phase: "Active", Labels: map[string]string{"app.kubernetes.io/part-of": "flux"},
			Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "ingress", Phase: "Active", Labels: map[string]string{"team": "platform"},
			Age: demoAge(4800), AgeSeconds: 4800 * 60},
	}
}

func demoServiceAccounts() []state.ServiceAccountView {
	return []state.ServiceAccountView{
		{Name: "default", Namespace: "default", AutomountToken: true, Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "api-gateway", Namespace: "default", Secrets: []string{"api-gateway-token"},
			ImagePullSecrets: []string{"registry-creds"}, AutomountToken: true, Age: demoAge(1500), AgeSeconds: 1500 * 60},
		{Name: "metrics-server", Namespace: "kube-system", AutomountToken: true,
			Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "prometheus", Namespace: "monitoring", Secrets: []string{"prometheus-token"},
			AutomountToken: true, Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoRoles() []state.RoleView {
	return []state.RoleView{
		{Name: "config-reader", Namespace: "default", Age: demoAge(320), AgeSeconds: 320 * 60,
			Rules: []state.PolicyRuleView{
				{APIGroups: []string{""}, Resources: []string{"configmaps", "secrets"}, Verbs: []string{"get", "list", "watch"}},
			}},
		{Name: "dashboard-reader", Namespace: "monitoring", Age: demoAge(2600), AgeSeconds: 2600 * 60,
			Rules: []state.PolicyRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "services"}, Verbs: []string{"get", "list", "watch"}},
				{APIGroups: []string{"apps"}, Resources: []string{"deployments"}, Verbs: []string{"get", "list"}},
			}},
	}
}

func demoClusterRoles() []state.ClusterRoleView {
	return []state.ClusterRoleView{
		{Name: "view", Age: demoAge(5000), AgeSeconds: 5000 * 60,
			Rules: []state.PolicyRuleView{
				{APIGroups: []string{""}, Resources: []string{"pods", "services", "configmaps"}, Verbs: []string{"get", "list", "watch"}},
			}},
		{Name: "crd-admin", Age: demoAge(1440), AgeSeconds: 1440 * 60,
			Rules: []state.PolicyRuleView{
				{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch"}},
			}},
	}
}

func demoRoleBindings() []state.RoleBindingView {
	return []state.RoleBindingView{
		{Name: "config-reader-binding", Namespace: "default", Age: demoAge(320), AgeSeconds: 320 * 60,
			RoleRef: state.RoleRefView{Kind: "Role", Name: "config-reader"},
			Subjects: []state.SubjectView{
				{Kind: "ServiceAccount", Name: "api-gateway", Namespace: "default"},
			}},
		{Name: "dashboard-reader-binding", Namespace: "monitoring", Age: demoAge(2600), AgeSeconds: 2600 * 60,
			RoleRef: state.RoleRefView{Kind: "Role", Name: "dashboard-reader"},
			Subjects: []state.SubjectView{
				{Kind: "Group", Name: "platform-observers"},
			}},
	}
}

func demoClusterRoleBindings() []state.ClusterRoleBindingView {
	return []state.ClusterRoleBindingView{
		{Name: "view-all", Age: demoAge(5000), AgeSeconds: 5000 * 60,
			RoleRef: state.RoleRefView{Kind: "ClusterRole", Name: "view"},
			Subjects: []state.SubjectView{
				{Kind: "Group", Name: "system:authenticated"},
			}},
		{Name: "crd-admins", Age: demoAge(1440), AgeSeconds: 1440 * 60,
			RoleRef: state.RoleRefView{Kind: "ClusterRole", Name: "crd-admin"},
			Subjects: []state.SubjectView{
				{Kind: "User", Name: "platform-admin@example.com"},
			}},
	}
}

func demoCustomResourceDefinitions() []state.CustomResourceDefinitionView {
	return []state.CustomResourceDefinitionView{
		{Name: "certificates.cert-manager.io", Group: "cert-manager.io", Scope: "Namespaced",
			Versions: []string{"v1"}, PluralName: "certificates", SingularName: "certificate",
			ListKind: "CertificateList", ShortNames: []string{"cert", "certs"}, Age: demoAge(3000), AgeSeconds: 3000 * 60},
		{Name: "prometheuses.monitoring.coreos.com", Group: "monitoring.coreos.com", Scope: "Namespaced",
			Versions: []string{"v1"}, PluralName: "prometheuses", SingularName: "prometheus",
			ListKind: "PrometheusList", ShortNames: []string{"prom"}, Age: demoAge(2600), AgeSeconds: 2600 * 60},
		{Name: "workflows.argoproj.io", Group: "argoproj.io", Scope: "Namespaced",
			Versions: []string{"v1alpha1"}, PluralName: "workflows", SingularName: "workflow",
			ListKind: "WorkflowList", ShortNames: []string{"wf"}, Age: demoAge(1800), AgeSeconds: 1800 * 60},
	}
}

func demoPersistentVolumeClaims() []state.PersistentVolumeClaimView {
	return []state.PersistentVolumeClaimView{
		{Name: "redis-data-cache-redis-0", Namespace: "default", Capacity: "10Gi", AccessModes: []string{"ReadWriteOnce"},
			StorageClassName: "fast-ssd", Phase: "bound", VolumeName: "pv-redis-0", Age: demoAge(4300), AgeSeconds: 4300 * 60},
		{Name: "prometheus-data-prometheus-0", Namespace: "monitoring", Capacity: "50Gi", AccessModes: []string{"ReadWriteOnce"},
			StorageClassName: "standard", Phase: "bound", VolumeName: "pv-prometheus-0", Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoPersistentVolumes() []state.PersistentVolumeView {
	return []state.PersistentVolumeView{
		{Name: "pv-redis-0", Capacity: "10Gi", AccessModes: []string{"ReadWriteOnce"}, ReclaimPolicy: "Delete",
			Phase: "bound", StorageClassName: "fast-ssd", ClaimNamespace: "default", ClaimName: "redis-data-cache-redis-0",
			Age: demoAge(4300), AgeSeconds: 4300 * 60},
		{Name: "pv-prometheus-0", Capacity: "50Gi", AccessModes: []string{"ReadWriteOnce"}, ReclaimPolicy: "Retain",
			Phase: "bound", StorageClassName: "standard", ClaimNamespace: "monitoring", ClaimName: "prometheus-data-prometheus-0",
			Age: demoAge(2600), AgeSeconds: 2600 * 60},
	}
}

func demoStorageClasses() []state.StorageClassView {
	return []state.StorageClassView{
		{Name: "standard", Provisioner: "kubernetes.io/no-provisioner", ReclaimPolicy: "Retain",
			VolumeBindingMode: "WaitForFirstConsumer", IsDefault: true, Age: demoAge(5000), AgeSeconds: 5000 * 60},
		{Name: "fast-ssd", Provisioner: "kubernetes.io/no-provisioner", ReclaimPolicy: "Delete",
			VolumeBindingMode: "Immediate", Age: demoAge(4300), AgeSeconds: 4300 * 60},
	}
}

// demoHelmReleases returns a small set of sample Helm releases so the
// HelmRelease list is populated in --demo mode.
func demoHelmReleases() []state.HelmReleaseView {
	return []state.HelmReleaseView{
		{Name: "ingress-nginx", Namespace: "ingress", Chart: "ingress-nginx",
			ChartVersion: "4.10.1", AppVersion: "1.9.0", Revision: 3,
			Status: "deployed", UpdatedAgeSec: 3600, AgeSeconds: 21 * 86400},
		{Name: "grafana", Namespace: "monitoring", Chart: "grafana",
			ChartVersion: "7.3.0", AppVersion: "10.4.2", Revision: 2,
			Status: "failed", UpdatedAgeSec: 9 * 3600, AgeSeconds: 14 * 86400},
		{Name: "cert-manager", Namespace: "kube-system", Chart: "cert-manager",
			ChartVersion: "1.14.4", AppVersion: "1.14.4", Revision: 1,
			Status: "deployed", UpdatedAgeSec: 30 * 86400, AgeSeconds: 30 * 86400},
	}
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
