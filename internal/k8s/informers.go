package k8s

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/jakenesler/kubagachi/internal/state"
)

// rebuildInterval debounces informer events: instead of rebuilding the whole
// snapshot on every individual change, the watcher coalesces changes and
// emits at most one snapshot per interval.
const rebuildInterval = 750 * time.Millisecond

// Watcher streams normalized ClusterState snapshots from a live cluster using
// shared informers over Pods, Nodes and Events, plus the Flux toolkit CRDs
// when they are installed.
type Watcher struct {
	client        *Client
	clientset     kubernetes.Interface
	clusterName   string
	namespace     string
	allNamespaces bool
	fluxResources []fluxResource

	metricsMu sync.RWMutex
	metricsOn bool
	nodeUsage map[string]usage
	podUsage  map[string]usage
}

// NewWatcher builds a Watcher. When allNamespaces is true the namespace
// filter is ignored and every namespace is watched.
func NewWatcher(c *Client, clusterName, namespace string, allNamespaces bool) *Watcher {
	return &Watcher{
		client:        c,
		clientset:     c.Clientset,
		clusterName:   clusterName,
		namespace:     namespace,
		allNamespaces: allNamespaces,
	}
}

// fluxPollInterval is how often the Flux CRDs are re-listed. Flux objects
// reconcile on the order of minutes, so a relaxed poll keeps API chatter low.
const fluxPollInterval = 5 * time.Second

// metricsPollInterval is how often node/pod usage is re-fetched from
// metrics.k8s.io. metrics-server aggregates on ~15s, so 10s keeps the bars
// live without hammering the API.
const metricsPollInterval = 10 * time.Second

// Run starts the informers and pushes a ClusterState snapshot onto out every
// time the cluster changes (debounced by rebuildInterval). It blocks until
// the informer caches sync, then returns; streaming continues in background
// goroutines until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context, out chan<- state.ClusterState) error {
	ns := w.namespace
	if w.allNamespaces {
		ns = "" // empty namespace == all namespaces for informers
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		w.clientset, 30*time.Second, informers.WithNamespace(ns),
	)
	podInformer := factory.Core().V1().Pods()
	nodeInformer := factory.Core().V1().Nodes()
	eventInformer := factory.Core().V1().Events()
	deploymentInformer := factory.Apps().V1().Deployments()
	serviceInformer := factory.Core().V1().Services()
	configMapInformer := factory.Core().V1().ConfigMaps()

	var dirty atomic.Bool
	dirty.Store(true)
	markDirty := cache.ResourceEventHandlerFuncs{
		AddFunc:    func(any) { dirty.Store(true) },
		UpdateFunc: func(any, any) { dirty.Store(true) },
		DeleteFunc: func(any) { dirty.Store(true) },
	}
	_, _ = podInformer.Informer().AddEventHandler(markDirty)
	_, _ = nodeInformer.Informer().AddEventHandler(markDirty)
	_, _ = eventInformer.Informer().AddEventHandler(markDirty)
	_, _ = deploymentInformer.Informer().AddEventHandler(markDirty)
	_, _ = serviceInformer.Informer().AddEventHandler(markDirty)
	_, _ = configMapInformer.Informer().AddEventHandler(markDirty)

	factory.Start(ctx.Done())
	for typ, ok := range factory.WaitForCacheSync(ctx.Done()) {
		if !ok {
			return fmt.Errorf("informer cache failed to sync: %v", typ)
		}
	}

	// Flux: discover the toolkit CRDs once, then keep a cached listing fresh
	// on a relaxed poll. Flux state changes mark the snapshot dirty like any
	// informer event would.
	w.fluxResources = discoverFlux(w.clientset)
	var fluxMu sync.RWMutex
	var fluxCache []state.FluxView
	if len(w.fluxResources) > 0 {
		fluxCache = listFlux(ctx, w.client.Dynamic, w.fluxResources, ns)
		go func() {
			ticker := time.NewTicker(fluxPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					next := listFlux(ctx, w.client.Dynamic, w.fluxResources, ns)
					fluxMu.Lock()
					changed := !fluxEqual(fluxCache, next)
					fluxCache = next
					fluxMu.Unlock()
					if changed {
						dirty.Store(true)
					}
				}
			}
		}()
	}

	// Metrics: poll node/pod usage from metrics.k8s.io when metrics-server is
	// installed. A change in usage marks the snapshot dirty so the bars stay
	// live; when absent, usage stays -1 and the UI shows "—".
	w.metricsOn = metricsAvailable(w.clientset)
	if w.metricsOn {
		refreshMetrics := func() {
			nu := listNodeMetrics(ctx, w.client.Dynamic)
			pu := listPodMetrics(ctx, w.client.Dynamic, ns)
			w.metricsMu.Lock()
			w.nodeUsage = nu
			w.podUsage = pu
			w.metricsMu.Unlock()
		}
		refreshMetrics()
		go func() {
			ticker := time.NewTicker(metricsPollInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					refreshMetrics()
					dirty.Store(true)
				}
			}
		}()
	}

	emit := func() {
		snap := w.build(
			podInformer.Lister(), nodeInformer.Lister(), eventInformer.Lister(),
			deploymentInformer.Lister(), serviceInformer.Lister(), configMapInformer.Lister(),
		)
		snap.FluxInstalled = len(w.fluxResources) > 0
		fluxMu.RLock()
		snap.Flux = append([]state.FluxView(nil), fluxCache...)
		fluxMu.RUnlock()
		snap.Rebuild()
		select {
		case out <- snap:
		case <-ctx.Done():
		}
	}
	emit() // initial snapshot

	go func() {
		defer close(out)
		ticker := time.NewTicker(rebuildInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if dirty.Swap(false) {
					emit()
				}
			}
		}
	}()
	return nil
}

// fluxEqual reports whether two flux listings are observably identical.
func fluxEqual(a, b []state.FluxView) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		// Age churns every poll; compare everything else.
		x, y := a[i], b[i]
		x.Age, y.Age = "", ""
		if x != y {
			return false
		}
	}
	return true
}

func (w *Watcher) build(
	podL listers.PodLister, nodeL listers.NodeLister, eventL listers.EventLister,
	deploymentL appslisters.DeploymentLister, serviceL listers.ServiceLister, configMapL listers.ConfigMapLister,
) state.ClusterState {
	snap := state.ClusterState{
		ClusterName:   w.clusterName,
		Namespace:     w.namespace,
		AllNamespaces: w.allNamespaces,
	}

	pods, _ := podL.List(labels.Everything())
	for _, p := range pods {
		snap.Pods = append(snap.Pods, MapPod(p))
	}
	nodes, _ := nodeL.List(labels.Everything())
	for _, n := range nodes {
		snap.Nodes = append(snap.Nodes, MapNode(n))
	}
	events, _ := eventL.List(labels.Everything())
	snap.Events = MapEvents(events)
	deployments, _ := deploymentL.List(labels.Everything())
	for _, d := range deployments {
		snap.Deployments = append(snap.Deployments, MapDeployment(d))
	}
	services, _ := serviceL.List(labels.Everything())
	for _, s := range services {
		snap.Services = append(snap.Services, MapService(s))
	}
	configMaps, _ := configMapL.List(labels.Everything())
	for _, c := range configMaps {
		snap.ConfigMaps = append(snap.ConfigMaps, MapConfigMap(c))
	}

	// Merge the latest usage sample (under lock) before Rebuild copies pods
	// into their node groups.
	w.metricsMu.RLock()
	snap.MetricsInstalled = w.metricsOn
	for i := range snap.Nodes {
		if u, ok := w.nodeUsage[snap.Nodes[i].Name]; ok {
			snap.Nodes[i].CPUUsedMilli = u.cpuMilli
			snap.Nodes[i].MemUsedBytes = u.memBytes
		}
	}
	for i := range snap.Pods {
		if u, ok := w.podUsage[snap.Pods[i].Namespace+"/"+snap.Pods[i].Name]; ok {
			snap.Pods[i].CPUUsedMilli = u.cpuMilli
			snap.Pods[i].MemUsedBytes = u.memBytes
		}
	}
	w.metricsMu.RUnlock()

	snap.Rebuild()
	return snap
}
