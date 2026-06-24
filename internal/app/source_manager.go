package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/jakenesler/kubagachi/internal/k8s"
	"github.com/jakenesler/kubagachi/internal/state"
	"github.com/jakenesler/kubagachi/internal/tui"
)

const demoContextName = "demo-cluster"

type sourceManager struct {
	cfg Config

	mu      sync.RWMutex
	source  ClusterSource
	cancel  context.CancelFunc
	current string
	gen     uint64

	out       chan state.ClusterState
	closeOnce sync.Once
}

func newSourceManager(ctx context.Context, cfg Config) (*sourceManager, <-chan state.ClusterState, error) {
	m := &sourceManager{
		cfg: cfg,
		out: make(chan state.ClusterState, 8),
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
	list, err := k8s.AvailableContexts()
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

func (m *sourceManager) selectContext(ctx context.Context, name string) error {
	if m.cfg.Demo {
		if name != "" && name != demoContextName {
			return fmt.Errorf("unknown context %q", name)
		}
		name = demoContextName
	} else {
		list, err := k8s.AvailableContexts()
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

	oldCancel := m.cancel
	m.source = source
	m.cancel = cancel
	m.current = current
	m.gen++
	gen := m.gen
	if oldCancel != nil {
		oldCancel()
	}
	go m.pipe(gen, stream)
	return nil
}

func (m *sourceManager) start(ctx context.Context, name string) (ClusterSource, context.CancelFunc, <-chan state.ClusterState, string, error) {
	cfg := m.cfg
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

func (a managedActions) ExecArgs(namespace, pod, container string) []string {
	return a.manager.currentActions().ExecArgs(namespace, pod, container)
}

func (a managedActions) FluxReconcile(ctx context.Context, kind, namespace, name string) error {
	return a.manager.currentActions().FluxReconcile(ctx, kind, namespace, name)
}

func (a managedActions) FluxSuspend(ctx context.Context, kind, namespace, name string, suspend bool) error {
	return a.manager.currentActions().FluxSuspend(ctx, kind, namespace, name, suspend)
}
