package k8s

import (
	"context"
	"log"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	appslisters "k8s.io/client-go/listers/apps/v1"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	listers "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/jakenesler/kubagachi/internal/state"
)

// rebuildInterval debounces informer events: instead of rebuilding the whole
// snapshot on every individual change, the watcher coalesces changes and
// emits at most one snapshot per interval.
const rebuildInterval = 750 * time.Millisecond

// cacheSyncTimeout bounds how long the watcher waits for informer caches to
// fill at startup. Past this it proceeds with whatever has synced so the
// cockpit comes up promptly; the dirty-driven rebuild loop backfills any
// informer that syncs later.
const cacheSyncTimeout = 30 * time.Second

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
	statefulSetInformer := factory.Apps().V1().StatefulSets()
	daemonSetInformer := factory.Apps().V1().DaemonSets()
	replicaSetInformer := factory.Apps().V1().ReplicaSets()
	jobInformer := factory.Batch().V1().Jobs()
	cronJobInformer := factory.Batch().V1().CronJobs()
	serviceInformer := factory.Core().V1().Services()
	ingressInformer := factory.Networking().V1().Ingresses()
	configMapInformer := factory.Core().V1().ConfigMaps()
	secretInformer := factory.Core().V1().Secrets()
	pvcInformer := factory.Core().V1().PersistentVolumeClaims()
	pvInformer := factory.Core().V1().PersistentVolumes()
	storageClassInformer := factory.Storage().V1().StorageClasses()

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
	_, _ = statefulSetInformer.Informer().AddEventHandler(markDirty)
	_, _ = daemonSetInformer.Informer().AddEventHandler(markDirty)
	_, _ = replicaSetInformer.Informer().AddEventHandler(markDirty)
	_, _ = jobInformer.Informer().AddEventHandler(markDirty)
	_, _ = cronJobInformer.Informer().AddEventHandler(markDirty)
	_, _ = serviceInformer.Informer().AddEventHandler(markDirty)
	_, _ = ingressInformer.Informer().AddEventHandler(markDirty)
	_, _ = configMapInformer.Informer().AddEventHandler(markDirty)
	_, _ = secretInformer.Informer().AddEventHandler(markDirty)
	_, _ = pvcInformer.Informer().AddEventHandler(markDirty)
	_, _ = pvInformer.Informer().AddEventHandler(markDirty)
	_, _ = storageClassInformer.Informer().AddEventHandler(markDirty)

	factory.Start(ctx.Done())
	// Degrade gracefully: a resource the cluster doesn't serve, or that our
	// RBAC can't list/watch, must not sink the whole watcher. Wait up to
	// cacheSyncTimeout, then proceed — any kind that failed to sync simply
	// renders as an empty list instead of blocking every other resource.
	syncCtx, cancelSync := context.WithTimeout(ctx, cacheSyncTimeout)
	for typ, ok := range factory.WaitForCacheSync(syncCtx.Done()) {
		if !ok {
			log.Printf("kubagachi: informer for %v did not sync within %s; continuing without it", typ, cacheSyncTimeout)
		}
	}
	cancelSync()

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
			deploymentInformer.Lister(), statefulSetInformer.Lister(), daemonSetInformer.Lister(),
			replicaSetInformer.Lister(), jobInformer.Lister(), cronJobInformer.Lister(),
			serviceInformer.Lister(), ingressInformer.Lister(), configMapInformer.Lister(),
			secretInformer.Lister(), pvcInformer.Lister(), pvInformer.Lister(), storageClassInformer.Lister(),
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
		// Age churns every poll; compare everything else. FluxView holds a
		// slice (DependsOn) so it is no longer comparable with ==.
		x, y := a[i], b[i]
		if x.Kind != y.Kind || x.Name != y.Name || x.Namespace != y.Namespace ||
			x.Ready != y.Ready || x.Suspended != y.Suspended || x.Revision != y.Revision ||
			x.Source != y.Source || x.Message != y.Message ||
			!slices.Equal(x.DependsOn, y.DependsOn) {
			return false
		}
	}
	return true
}

func (w *Watcher) build(
	podL listers.PodLister, nodeL listers.NodeLister, eventL listers.EventLister,
	deploymentL appslisters.DeploymentLister, statefulSetL appslisters.StatefulSetLister,
	daemonSetL appslisters.DaemonSetLister, replicaSetL appslisters.ReplicaSetLister,
	jobL batchlisters.JobLister, cronJobL batchlisters.CronJobLister,
	serviceL listers.ServiceLister, ingressL networkinglisters.IngressLister, configMapL listers.ConfigMapLister,
	secretL listers.SecretLister, pvcL listers.PersistentVolumeClaimLister,
	pvL listers.PersistentVolumeLister, storageClassL storagelisters.StorageClassLister,
) state.ClusterState {
	snap := state.ClusterState{
		ClusterName:   w.clusterName,
		ServerVersion: w.client.ServerVersion,
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
	statefulSets, _ := statefulSetL.List(labels.Everything())
	for _, s := range statefulSets {
		snap.StatefulSets = append(snap.StatefulSets, MapStatefulSet(s))
	}
	daemonSets, _ := daemonSetL.List(labels.Everything())
	for _, d := range daemonSets {
		snap.DaemonSets = append(snap.DaemonSets, MapDaemonSet(d))
	}
	replicaSets, _ := replicaSetL.List(labels.Everything())
	for _, r := range replicaSets {
		snap.ReplicaSets = append(snap.ReplicaSets, MapReplicaSet(r))
	}
	jobs, _ := jobL.List(labels.Everything())
	for _, j := range jobs {
		snap.Jobs = append(snap.Jobs, MapJob(j))
	}
	cronJobs, _ := cronJobL.List(labels.Everything())
	for _, c := range cronJobs {
		snap.CronJobs = append(snap.CronJobs, MapCronJob(c))
	}
	services, _ := serviceL.List(labels.Everything())
	for _, s := range services {
		snap.Services = append(snap.Services, MapService(s))
	}
	ingresses, _ := ingressL.List(labels.Everything())
	for _, i := range ingresses {
		snap.Ingresses = append(snap.Ingresses, MapIngress(i))
	}
	configMaps, _ := configMapL.List(labels.Everything())
	for _, c := range configMaps {
		snap.ConfigMaps = append(snap.ConfigMaps, MapConfigMap(c))
	}
	secrets, _ := secretL.List(labels.Everything())
	for _, s := range secrets {
		snap.Secrets = append(snap.Secrets, MapSecret(s))
	}
	pvcs, _ := pvcL.List(labels.Everything())
	for _, p := range pvcs {
		snap.PersistentVolumeClaims = append(snap.PersistentVolumeClaims, MapPersistentVolumeClaim(p))
	}
	pvs, _ := pvL.List(labels.Everything())
	for _, p := range pvs {
		snap.PersistentVolumes = append(snap.PersistentVolumes, MapPersistentVolume(p))
	}
	storageClasses, _ := storageClassL.List(labels.Everything())
	for _, s := range storageClasses {
		snap.StorageClasses = append(snap.StorageClasses, MapStorageClass(s))
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
