package app

import (
	"context"
	"fmt"

	"github.com/yscale-sh/kubagachi/internal/k8s"
	"github.com/yscale-sh/kubagachi/internal/state"
	"github.com/yscale-sh/kubagachi/internal/tui"
)

// ClusterSource is anything that streams normalized cluster snapshots. Both
// the live Kubernetes watcher and the demo data generator implement it, which
// keeps the TUI fully decoupled from where its data comes from.
//
// Snapshots are delivered over a Go channel so the TUI never blocks on I/O:
// it simply selects on the channel inside a Bubble Tea command.
type ClusterSource interface {
	// Stream starts producing snapshots and returns a receive-only channel.
	// Streaming continues until ctx is cancelled, at which point the channel
	// is closed. It returns an error if the source cannot start.
	Stream(ctx context.Context) (<-chan state.ClusterState, error)

	// Label is a short human-readable name for the source.
	Label() string

	// Actions exposes the cluster side-effect surface (logs, describe,
	// delete, shell, flux reconcile/suspend).
	Actions() tui.Actions
}

// selectSource picks the data source implied by the CLI configuration.
func selectSource(cfg Config) (ClusterSource, error) {
	if cfg.Demo {
		return demoSource{cfg: cfg}, nil
	}
	return newClusterSource(cfg)
}

// clusterSource streams snapshots from a real Kubernetes cluster via informers.
type clusterSource struct {
	client    *k8s.Client
	namespace string
	allNS     bool
}

// newClusterSource connects to the cluster up front so connection errors are
// reported before the TUI takes over the terminal.
func newClusterSource(cfg Config) (*clusterSource, error) {
	src := k8s.KubeconfigSource{Path: cfg.KubeconfigPath, Raw: cfg.KubeconfigRaw}
	client, err := k8s.NewClient(src, cfg.Context)
	if err != nil {
		return nil, err
	}
	ns := cfg.Namespace
	if ns == "" {
		ns = client.DefaultNamespace
	}
	return &clusterSource{
		client:    client,
		namespace: ns,
		allNS:     cfg.AllNamespaces,
	}, nil
}

func (s *clusterSource) Label() string { return "cluster" }

func (s *clusterSource) Stream(ctx context.Context) (<-chan state.ClusterState, error) {
	out := make(chan state.ClusterState, 4)
	watcher := k8s.NewWatcher(s.client, s.client.ContextName, s.namespace, s.allNS)
	if err := watcher.Run(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *clusterSource) Actions() tui.Actions { return liveActions{client: s.client} }

// liveActions adapts *k8s.Client to the tui.Actions interface.
type liveActions struct {
	client *k8s.Client
}

func (a liveActions) Logs(ctx context.Context, namespace, pod, container string, tail int64) (string, error) {
	return a.client.PodLogs(ctx, namespace, pod, container, tail)
}

func (a liveActions) Describe(ctx context.Context, namespace, pod string) (string, error) {
	return a.client.Describe(ctx, namespace, pod)
}

func (a liveActions) DeletePod(ctx context.Context, namespace, pod string) error {
	return a.client.DeletePod(ctx, namespace, pod)
}

func (a liveActions) DeleteResource(ctx context.Context, apiVersion, kind, namespace, name string) error {
	return a.client.DeleteResource(ctx, apiVersion, kind, namespace, name)
}

func (a liveActions) ScaleResource(ctx context.Context, apiVersion, kind, namespace, name string, replicas int32) error {
	return a.client.ScaleResource(ctx, apiVersion, kind, namespace, name, replicas)
}

func (a liveActions) RestartResource(ctx context.Context, apiVersion, kind, namespace, name string) error {
	return a.client.RestartResource(ctx, apiVersion, kind, namespace, name)
}

func (a liveActions) ExecArgs(namespace, pod, container string) []string {
	return a.client.ExecArgs(namespace, pod, container)
}

func (a liveActions) FluxReconcile(ctx context.Context, kind, namespace, name string) error {
	return a.client.FluxReconcile(ctx, kind, namespace, name)
}

func (a liveActions) FluxSuspend(ctx context.Context, kind, namespace, name string, suspend bool) error {
	return a.client.FluxSuspend(ctx, kind, namespace, name, suspend)
}

func (a liveActions) ObjectYAML(ctx context.Context, apiVersion, kind, namespace, name string) (string, error) {
	return a.client.ObjectYAML(ctx, apiVersion, kind, namespace, name)
}

func (a liveActions) CustomResources(ctx context.Context, group, version, resource, namespace string) ([]tui.CustomResourceRef, error) {
	raw, err := a.client.CustomResources(ctx, group, version, resource, namespace)
	if err != nil {
		return nil, err
	}
	out := make([]tui.CustomResourceRef, 0, len(raw))
	for _, r := range raw {
		out = append(out, tui.CustomResourceRef{
			Name:      r.Name,
			Namespace: r.Namespace,
			AgeSec:    r.AgeSec,
		})
	}
	return out, nil
}

func (a liveActions) ApplyYAML(ctx context.Context, yaml string) (string, error) {
	return a.client.ApplyYAML(ctx, yaml)
}

func (a liveActions) SecretData(ctx context.Context, namespace, name string) (map[string]tui.SecretValue, error) {
	raw, err := a.client.SecretData(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	out := make(map[string]tui.SecretValue, len(raw))
	for k, v := range raw {
		out[k] = tui.SecretValue{B64: v.B64, Decoded: v.Decoded}
	}
	return out, nil
}

func (a liveActions) CordonNode(ctx context.Context, name string, cordon bool) error {
	return a.client.CordonNode(ctx, name, cordon)
}

func (a liveActions) PortForwardStart(ctx context.Context, namespace, pod string, remotePort, localPort int) (tui.PortForward, error) {
	info, err := a.client.PortForwardStart(ctx, namespace, pod, remotePort, localPort)
	if err != nil {
		return tui.PortForward{}, err
	}
	return tui.PortForward{
		ID:         info.ID,
		Namespace:  info.Namespace,
		Pod:        info.Pod,
		RemotePort: info.RemotePort,
		LocalPort:  info.LocalPort,
		AgeSec:     info.AgeSec,
	}, nil
}

func (a liveActions) PortForwardStop(id string) error {
	return a.client.PortForwardStop(id)
}

func (a liveActions) PortForwardList() []tui.PortForward {
	raw := a.client.PortForwardList()
	out := make([]tui.PortForward, 0, len(raw))
	for _, info := range raw {
		out = append(out, tui.PortForward{
			ID:         info.ID,
			Namespace:  info.Namespace,
			Pod:        info.Pod,
			RemotePort: info.RemotePort,
			LocalPort:  info.LocalPort,
			AgeSec:     info.AgeSec,
		})
	}
	return out
}

// PortForwardStopAll stops every active forward on the underlying client.
// Exposed for the context-switch seam.
func (a liveActions) PortForwardStopAll() {
	a.client.PortForwardStopAll()
}

func (a liveActions) HelmHistory(ctx context.Context, namespace, name string) ([]tui.HelmRevision, error) {
	return a.client.HelmHistory(ctx, namespace, name)
}

func (a liveActions) HelmReleaseDetail(ctx context.Context, namespace, name string, revision int) (tui.HelmDetail, error) {
	return a.client.HelmReleaseDetail(ctx, namespace, name, revision)
}

func (a liveActions) HelmAvailable() bool {
	return a.client.HelmAvailable()
}

func (a liveActions) HelmRollback(ctx context.Context, namespace, name string, revision int) (string, error) {
	return a.client.HelmRollback(ctx, namespace, name, revision)
}

func (a liveActions) HelmUninstall(ctx context.Context, namespace, name string) (string, error) {
	return a.client.HelmUninstall(ctx, namespace, name)
}

// demoActions answers every action with a friendly stub so the demo stays
// self-contained.
type demoActions struct{}

func (demoActions) Logs(_ context.Context, namespace, pod, _ string, _ int64) (string, error) {
	return fmt.Sprintf(`demo logs for %s/%s

12:00:01 INF critter awake — listening on :8080
12:00:02 INF purring at 60rpm
12:00:05 INF handled 42 requests
12:00:09 WRN treat supply low
12:00:12 INF napping between requests…

(run against a real cluster for live logs)`, namespace, pod), nil
}

func (demoActions) Describe(_ context.Context, namespace, pod string) (string, error) {
	return fmt.Sprintf(`name             %s
namespace        %s
node             critter-node-a
status           Running

(this is demo data — connect to a real cluster for the full describe)`, pod, namespace), nil
}

func (demoActions) DeletePod(_ context.Context, _, _ string) error {
	return fmt.Errorf("demo pods are protected wildlife")
}

func (demoActions) DeleteResource(_ context.Context, _, _, _, _ string) error { return nil }

func (demoActions) ScaleResource(_ context.Context, _, _, _, _ string, _ int32) error { return nil }

func (demoActions) RestartResource(_ context.Context, _, _, _, _ string) error { return nil }

func (demoActions) ExecArgs(_, _, _ string) []string { return nil }

func (demoActions) FluxReconcile(_ context.Context, _, _, _ string) error {
	return nil // pretend it worked — demo flux objects refresh on their own
}

func (demoActions) FluxSuspend(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}

func (demoActions) ApplyYAML(_ context.Context, yaml string) (string, error) {
	// Demo has no cluster; return the input unchanged.
	return yaml, nil
}

func (demoActions) ObjectYAML(_ context.Context, apiVersion, kind, namespace, name string) (string, error) {
	ns := namespace
	if ns == "" {
		ns = "default"
	}
	av := apiVersion
	if av == "" {
		av = "v1"
	}
	return fmt.Sprintf(`apiVersion: %s
kind: %s
metadata:
  name: %s
  namespace: %s
  labels:
    app: %s
spec:
  replicas: 1
  selector:
    matchLabels:
      app: %s
status:
  phase: Running
  conditions:
    - type: Ready
      status: "True"
`, av, kind, name, ns, name, name), nil
}

func (demoActions) CustomResources(_ context.Context, _, _, resource, namespace string) ([]tui.CustomResourceRef, error) {
	prefix := resource
	if prefix == "" {
		prefix = "customresource"
	}
	return []tui.CustomResourceRef{
		{Name: prefix + "-alpha", Namespace: namespace, AgeSec: 420},
		{Name: prefix + "-bravo", Namespace: namespace, AgeSec: 3600},
		{Name: prefix + "-charlie", Namespace: namespace, AgeSec: 86400},
	}, nil
}

func (demoActions) SecretData(_ context.Context, _, _ string) (map[string]tui.SecretValue, error) {
	return map[string]tui.SecretValue{
		"username": {B64: "YWRtaW4=", Decoded: "admin"},
		"password": {B64: "czNjcmV0", Decoded: "s3cret"},
	}, nil
}

func (demoActions) CordonNode(_ context.Context, _ string, _ bool) error { return nil }

func (demoActions) PortForwardStart(_ context.Context, _, _ string, _, _ int) (tui.PortForward, error) {
	return tui.PortForward{}, fmt.Errorf("port-forward is unavailable in demo mode")
}

func (demoActions) PortForwardStop(_ string) error { return nil }

func (demoActions) PortForwardList() []tui.PortForward { return []tui.PortForward{} }

func (demoActions) HelmAvailable() bool { return false }

func (demoActions) HelmRollback(_ context.Context, _, _ string, _ int) (string, error) {
	return "", fmt.Errorf("helm actions are unavailable in demo mode")
}

func (demoActions) HelmUninstall(_ context.Context, _, _ string) (string, error) {
	return "", fmt.Errorf("helm actions are unavailable in demo mode")
}

func (demoActions) HelmHistory(_ context.Context, _, _ string) ([]tui.HelmRevision, error) {
	return []tui.HelmRevision{
		{Revision: 3, Status: "deployed", ChartVersion: "4.10.1", AppVersion: "1.9.0", UpdatedAgeSec: 3600, Description: "Upgrade complete"},
		{Revision: 2, Status: "superseded", ChartVersion: "4.9.0", AppVersion: "1.8.5", UpdatedAgeSec: 86400, Description: "Upgrade complete"},
		{Revision: 1, Status: "superseded", ChartVersion: "4.8.0", AppVersion: "1.8.0", UpdatedAgeSec: 7 * 86400, Description: "Install complete"},
	}, nil
}

func (demoActions) HelmReleaseDetail(_ context.Context, _, _ string, _ int) (tui.HelmDetail, error) {
	return tui.HelmDetail{
		Values:   "replicaCount: 2\nservice:\n  type: ClusterIP\n  port: 80\nresources:\n  requests:\n    cpu: 100m\n    memory: 128Mi\n",
		Manifest: "---\n# Source: chart/templates/deployment.yaml\napiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: demo-release\nspec:\n  replicas: 2\n",
		Notes:    "Chart deployed successfully.\n\nGet the application URL:\n  kubectl get svc --namespace {{ .Release.Namespace }} {{ .Release.Name }}\n",
	}, nil
}
