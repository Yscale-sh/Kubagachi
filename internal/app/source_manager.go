package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/yscale-sh/kubagachi/internal/k8s"
	"github.com/yscale-sh/kubagachi/internal/state"
	"github.com/yscale-sh/kubagachi/internal/tui"
)

const demoContextName = "demo-cluster"

type sourceManager struct {
	cfg Config

	mu      sync.RWMutex
	source  ClusterSource
	cancel  context.CancelFunc
	current string
	gen     uint64
	// kube is the live kubeconfig source; it starts from cfg and can be swapped
	// at runtime via setKubeconfig (the web settings panel).
	kube k8s.KubeconfigSource

	out       chan state.ClusterState
	closeOnce sync.Once
}

func newSourceManager(ctx context.Context, cfg Config) (*sourceManager, <-chan state.ClusterState, error) {
	m := &sourceManager{
		cfg:  cfg,
		kube: k8s.KubeconfigSource{Path: cfg.KubeconfigPath, Raw: cfg.KubeconfigRaw},
		out:  make(chan state.ClusterState, 8),
	}
	if err := m.selectContext(ctx, cfg.Context); err != nil {
		return nil, nil, err
	}
	go func() {
		<-ctx.Done()
		m.close()
	}()
	return m, m.out, nil
}

func (m *sourceManager) Label() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.source == nil {
		return "cluster"
	}
	return m.source.Label()
}

func (m *sourceManager) Stream(_ context.Context) (<-chan state.ClusterState, error) {
	return m.out, nil
}

func (m *sourceManager) Actions() tui.Actions {
	return managedActions{manager: m}
}

func (m *sourceManager) contexts() (k8s.ContextList, error) {
	if m.cfg.Demo {
		return k8s.ContextList{
			Current: demoContextName,
			Contexts: []k8s.ContextInfo{{
				Name:      demoContextName,
				Cluster:   demoContextName,
				Namespace: m.cfg.Namespace,
			}},
		}, nil
	}
	list, err := k8s.AvailableContexts(m.kubeSource())
	if err != nil {
		return k8s.ContextList{}, err
	}
	m.mu.RLock()
	if m.current != "" {
		list.Current = m.current
	}
	m.mu.RUnlock()
	return list, nil
}

// kubeSource returns a snapshot of the live kubeconfig source. Callers must not
// already hold m.mu (it takes a read lock).
func (m *sourceManager) kubeSource() k8s.KubeconfigSource {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.kube
}

func (m *sourceManager) selectContext(ctx context.Context, name string) error {
	if m.cfg.Demo {
		if name != "" && name != demoContextName {
			return fmt.Errorf("unknown context %q", name)
		}
		name = demoContextName
	} else {
		list, err := k8s.AvailableContexts(m.kubeSource())
		if err != nil {
			return err
		}
		if name == "" {
			name = list.Current
		}
		if !contextExists(list.Contexts, name) {
			return fmt.Errorf("unknown context %q", name)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if name == m.current && m.source != nil {
		return nil
	}

	source, cancel, stream, current, err := m.start(ctx, name)
	if err != nil {
		return err
	}

	oldSource := m.source
	oldCancel := m.cancel
	m.source = source
	m.cancel = cancel
	m.current = current
	m.gen++
	gen := m.gen
	// Stop any active port-forwards on the old source before switching away.
	if oldSource != nil {
		if stopper, ok := oldSource.Actions().(interface{ PortForwardStopAll() }); ok {
			stopper.PortForwardStopAll()
		}
	}
	if oldCancel != nil {
		oldCancel()
	}
	go m.pipe(gen, stream)
	return nil
}

// setKubeconfig swaps the live kubeconfig source (a pasted config or an explicit
// file path) and switches to its current-context. It validates that the config
// parses and exposes at least one context before touching the live source, and
// rolls back if connecting to the new cluster fails — so a bad config leaves the
// running cockpit untouched. A successful swap also leaves demo mode.
func (m *sourceManager) setKubeconfig(ctx context.Context, src k8s.KubeconfigSource) (k8s.ContextList, error) {
	list, err := k8s.AvailableContexts(src)
	if err != nil {
		return k8s.ContextList{}, err
	}
	if len(list.Contexts) == 0 {
		return k8s.ContextList{}, fmt.Errorf("kubeconfig has no contexts")
	}

	m.mu.Lock()
	prevKube, prevDemo, prevCurrent := m.kube, m.cfg.Demo, m.current
	m.kube = src
	m.cfg.Demo = false // a plugged-in kubeconfig means we go live
	m.current = ""     // force a fresh switch to the new config's current-context
	m.mu.Unlock()

	if err := m.selectContext(ctx, ""); err != nil {
		m.mu.Lock()
		m.kube, m.cfg.Demo, m.current = prevKube, prevDemo, prevCurrent
		m.mu.Unlock()
		return k8s.ContextList{}, err
	}
	return m.contexts()
}

func (m *sourceManager) start(ctx context.Context, name string) (ClusterSource, context.CancelFunc, <-chan state.ClusterState, string, error) {
	// Callers hold m.mu, so read m.kube directly rather than via kubeSource().
	cfg := m.cfg
	cfg.KubeconfigPath = m.kube.Path
	cfg.KubeconfigRaw = m.kube.Raw
	if !cfg.Demo {
		cfg.Context = name
	}
	source, err := selectSource(cfg)
	if err != nil {
		return nil, nil, nil, "", err
	}
	childCtx, cancel := context.WithCancel(ctx)
	stream, err := source.Stream(childCtx)
	if err != nil {
		cancel()
		return nil, nil, nil, "", err
	}
	current := name
	if c, ok := source.(*clusterSource); ok {
		current = c.client.ContextName
	}
	if current == "" && cfg.Demo {
		current = demoContextName
	}
	return source, cancel, stream, current, nil
}

func (m *sourceManager) pipe(gen uint64, stream <-chan state.ClusterState) {
	for snap := range stream {
		m.mu.RLock()
		current := gen == m.gen
		m.mu.RUnlock()
		if !current {
			continue
		}
		select {
		case m.out <- snap:
		default:
			<-m.out
			m.out <- snap
		}
	}
}

func (m *sourceManager) close() {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		if m.cancel != nil {
			m.cancel()
		}
		m.cancel = nil
		m.source = nil
		m.mu.Unlock()
		// Deliberately do not close(m.out): a live pipe goroutine may still be
		// mid-send after cancel(), and closing would panic it. Cancelling the
		// source stops the upstream; the web consumer exits on ctx.Done.
	})
}

func (m *sourceManager) currentActions() tui.Actions {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.source == nil {
		return demoActions{}
	}
	return m.source.Actions()
}

func contextExists(contexts []k8s.ContextInfo, name string) bool {
	for _, ctx := range contexts {
		if ctx.Name == name {
			return true
		}
	}
	return false
}

type managedActions struct {
	manager *sourceManager
}

func (a managedActions) Logs(ctx context.Context, namespace, pod, container string, tail int64) (string, error) {
	return a.manager.currentActions().Logs(ctx, namespace, pod, container, tail)
}

func (a managedActions) Describe(ctx context.Context, namespace, pod string) (string, error) {
	return a.manager.currentActions().Describe(ctx, namespace, pod)
}

func (a managedActions) DeletePod(ctx context.Context, namespace, pod string) error {
	return a.manager.currentActions().DeletePod(ctx, namespace, pod)
}

func (a managedActions) DeleteResource(ctx context.Context, apiVersion, kind, namespace, name string) error {
	return a.manager.currentActions().DeleteResource(ctx, apiVersion, kind, namespace, name)
}

func (a managedActions) ScaleResource(ctx context.Context, apiVersion, kind, namespace, name string, replicas int32) error {
	return a.manager.currentActions().ScaleResource(ctx, apiVersion, kind, namespace, name, replicas)
}

func (a managedActions) RestartResource(ctx context.Context, apiVersion, kind, namespace, name string) error {
	return a.manager.currentActions().RestartResource(ctx, apiVersion, kind, namespace, name)
}

func (a managedActions) ExecArgs(namespace, pod, container string) []string {
	return a.manager.currentActions().ExecArgs(namespace, pod, container)
}

func (a managedActions) FluxReconcile(ctx context.Context, kind, namespace, name string) error {
	return a.manager.currentActions().FluxReconcile(ctx, kind, namespace, name)
}

func (a managedActions) FluxSuspend(ctx context.Context, kind, namespace, name string, suspend bool) error {
	return a.manager.currentActions().FluxSuspend(ctx, kind, namespace, name, suspend)
}

func (a managedActions) ObjectYAML(ctx context.Context, apiVersion, kind, namespace, name string) (string, error) {
	return a.manager.currentActions().ObjectYAML(ctx, apiVersion, kind, namespace, name)
}

func (a managedActions) CustomResources(ctx context.Context, group, version, resource, namespace string) ([]tui.CustomResourceRef, error) {
	return a.manager.currentActions().CustomResources(ctx, group, version, resource, namespace)
}

func (a managedActions) ApplyYAML(ctx context.Context, yaml string) (string, error) {
	return a.manager.currentActions().ApplyYAML(ctx, yaml)
}

func (a managedActions) SecretData(ctx context.Context, namespace, name string) (map[string]tui.SecretValue, error) {
	return a.manager.currentActions().SecretData(ctx, namespace, name)
}

func (a managedActions) CordonNode(ctx context.Context, name string, cordon bool) error {
	return a.manager.currentActions().CordonNode(ctx, name, cordon)
}

func (a managedActions) PortForwardStart(ctx context.Context, namespace, pod string, remotePort, localPort int) (tui.PortForward, error) {
	return a.manager.currentActions().PortForwardStart(ctx, namespace, pod, remotePort, localPort)
}

func (a managedActions) PortForwardStop(id string) error {
	return a.manager.currentActions().PortForwardStop(id)
}

func (a managedActions) PortForwardList() []tui.PortForward {
	return a.manager.currentActions().PortForwardList()
}

func (a managedActions) HelmHistory(ctx context.Context, namespace, name string) ([]tui.HelmRevision, error) {
	return a.manager.currentActions().HelmHistory(ctx, namespace, name)
}

func (a managedActions) HelmReleaseDetail(ctx context.Context, namespace, name string, revision int) (tui.HelmDetail, error) {
	return a.manager.currentActions().HelmReleaseDetail(ctx, namespace, name, revision)
}

func (a managedActions) HelmAvailable() bool {
	return a.manager.currentActions().HelmAvailable()
}

func (a managedActions) HelmRollback(ctx context.Context, namespace, name string, revision int) (string, error) {
	return a.manager.currentActions().HelmRollback(ctx, namespace, name, revision)
}

func (a managedActions) HelmUninstall(ctx context.Context, namespace, name string) (string, error) {
	return a.manager.currentActions().HelmUninstall(ctx, namespace, name)
}
