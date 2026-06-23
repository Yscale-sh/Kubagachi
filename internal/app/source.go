package app

import (
	"context"
	"fmt"

	"github.com/jakenesler/kubagachi/internal/k8s"
	"github.com/jakenesler/kubagachi/internal/state"
	"github.com/jakenesler/kubagachi/internal/tui"
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
	client, err := k8s.NewClient(cfg.Context)
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

func (a liveActions) ExecArgs(namespace, pod, container string) []string {
	return a.client.ExecArgs(namespace, pod, container)
}

func (a liveActions) FluxReconcile(ctx context.Context, kind, namespace, name string) error {
	return a.client.FluxReconcile(ctx, kind, namespace, name)
}

func (a liveActions) FluxSuspend(ctx context.Context, kind, namespace, name string, suspend bool) error {
	return a.client.FluxSuspend(ctx, kind, namespace, name, suspend)
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

func (demoActions) ExecArgs(_, _, _ string) []string { return nil }

func (demoActions) FluxReconcile(_ context.Context, _, _, _ string) error {
	return nil // pretend it worked — demo flux objects refresh on their own
}

func (demoActions) FluxSuspend(_ context.Context, _, _, _ string, _ bool) error {
	return nil
}
