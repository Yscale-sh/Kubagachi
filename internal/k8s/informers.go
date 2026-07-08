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
	autoscalinglisters "k8s.io/client-go/listers/autoscaling/v2"
	batchlisters "k8s.io/client-go/listers/batch/v1"
	listers "k8s.io/client-go/listers/core/v1"
	networkinglisters "k8s.io/client-go/listers/networking/v1"
	policylisters "k8s.io/client-go/listers/policy/v1"
	rbaclisters "k8s.io/client-go/listers/rbac/v1"
	storagelisters "k8s.io/client-go/listers/storage/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/yscale-sh/kubagachi/internal/state"
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
	endpointInformer := factory.Core().V1().Endpoints()
	networkPolicyInformer := factory.Networking().V1().NetworkPolicies()
	configMapInformer := factory.Core().V1().ConfigMaps()
	secretInformer := factory.Core().V1().Secrets()
	resourceQuotaInformer := factory.Core().V1().ResourceQuotas()
	limitRangeInformer := factory.Core().V1().LimitRanges()
	hpaInformer := factory.Autoscaling().V2().HorizontalPodAutoscalers()
	pdbInformer := factory.Policy().V1().PodDisruptionBudgets()
	serviceAccountInformer := factory.Core().V1().ServiceAccounts()
	namespaceInformer := factory.Core().V1().Namespaces()
	roleInformer := factory.Rbac().V1().Roles()
	clusterRoleInformer := factory.Rbac().V1().ClusterRoles()
	roleBindingInformer := factory.Rbac().V1().RoleBindings()
	clusterRoleBindingInformer := factory.Rbac().V1().ClusterRoleBindings()
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
	_, _ = endpointInformer.Informer().AddEventHandler(markDirty)
	_, _ = networkPolicyInformer.Informer().AddEventHandler(markDirty)
	_, _ = configMapInformer.Informer().AddEventHandler(markDirty)
	_, _ = secretInformer.Informer().AddEventHandler(markDirty)
	_, _ = resourceQuotaInformer.Informer().AddEventHandler(markDirty)
	_, _ = limitRangeInformer.Informer().AddEventHandler(markDirty)
	_, _ = hpaInformer.Informer().AddEventHandler(markDirty)
	_, _ = pdbInformer.Informer().AddEventHandler(markDirty)
	_, _ = serviceAccountInformer.Informer().AddEventHandler(markDirty)
	_, _ = namespaceInformer.Informer().AddEventHandler(markDirty)
	_, _ = roleInformer.Informer().AddEventHandler(markDirty)
	_, _ = clusterRoleInformer.Informer().AddEventHandler(markDirty)
	_, _ = roleBindingInformer.Informer().AddEventHandler(markDirty)
	_, _ = clusterRoleBindingInformer.Informer().AddEventHandler(markDirty)
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

	// CRDs are listed through the dynamic client to avoid a separate
	// apiextensions clientset dependency. They are cluster-scoped, so the
	// namespace filter does not apply.
	var crdMu sync.RWMutex
	crdCache := listCustomResourceDefinitions(ctx, w.client.Dynamic)
	go func() {
		ticker := time.NewTicker(fluxPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next := listCustomResourceDefinitions(ctx, w.client.Dynamic)
				crdMu.Lock()
				changed := !customResourceDefinitionsEqual(crdCache, next)
				crdCache = next
				crdMu.Unlock()
				if changed {
					dirty.Store(true)
				}
			}
		}
	}()

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
			endpointInformer.Lister(), networkPolicyInformer.Lister(), secretInformer.Lister(),
			resourceQuotaInformer.Lister(), limitRangeInformer.Lister(), hpaInformer.Lister(),
			pdbInformer.Lister(), serviceAccountInformer.Lister(), namespaceInformer.Lister(),
			roleInformer.Lister(), clusterRoleInformer.Lister(), roleBindingInformer.Lister(),
			clusterRoleBindingInformer.Lister(), pvcInformer.Lister(), pvInformer.Lister(),
			storageClassInformer.Lister(),
		)
		snap.FluxInstalled = len(w.fluxResources) > 0
		fluxMu.RLock()
		snap.Flux = append([]state.FluxView(nil), fluxCache...)
		fluxMu.RUnlock()
		crdMu.RLock()
		snap.CustomResourceDefinitions = append([]state.CustomResourceDefinitionView(nil), crdCache...)
		crdMu.RUnlock()
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
	endpointL listers.EndpointsLister, networkPolicyL networkinglisters.NetworkPolicyLister,
	secretL listers.SecretLister, resourceQuotaL listers.ResourceQuotaLister, limitRangeL listers.LimitRangeLister,
	hpaL autoscalinglisters.HorizontalPodAutoscalerLister, pdbL policylisters.PodDisruptionBudgetLister,
	serviceAccountL listers.ServiceAccountLister, namespaceL listers.NamespaceLister,
	roleL rbaclisters.RoleLister, clusterRoleL rbaclisters.ClusterRoleLister,
	roleBindingL rbaclisters.RoleBindingLister, clusterRoleBindingL rbaclisters.ClusterRoleBindingLister,
	pvcL listers.PersistentVolumeClaimLister,
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
	namespaces, _ := namespaceL.List(labels.Everything())
	for _, n := range namespaces {
		if !w.allNamespaces && w.namespace != "" && n.Name != w.namespace {
			continue
		}
		snap.Namespaces = append(snap.Namespaces, MapNamespace(n))
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
	endpoints, _ := endpointL.List(labels.Everything())
	for _, e := range endpoints {
		snap.Endpoints = append(snap.Endpoints, MapEndpoint(e))
	}
	networkPolicies, _ := networkPolicyL.List(labels.Everything())
	for _, n := range networkPolicies {
		snap.NetworkPolicies = append(snap.NetworkPolicies, MapNetworkPolicy(n))
	}
	configMaps, _ := configMapL.List(labels.Everything())
	for _, c := range configMaps {
		snap.ConfigMaps = append(snap.ConfigMaps, MapConfigMap(c))
	}
	secrets, _ := secretL.List(labels.Everything())
	for _, s := range secrets {
		snap.Secrets = append(snap.Secrets, MapSecret(s))
	}
	snap.HelmReleases = MapHelmReleases(secrets)
	resourceQuotas, _ := resourceQuotaL.List(labels.Everything())
	for _, r := range resourceQuotas {
		snap.ResourceQuotas = append(snap.ResourceQuotas, MapResourceQuota(r))
	}
	limitRanges, _ := limitRangeL.List(labels.Everything())
	for _, l := range limitRanges {
		snap.LimitRanges = append(snap.LimitRanges, MapLimitRange(l))
	}
	hpas, _ := hpaL.List(labels.Everything())
	for _, h := range hpas {
		snap.HorizontalPodAutoscalers = append(snap.HorizontalPodAutoscalers, MapHorizontalPodAutoscaler(h))
	}
	pdbs, _ := pdbL.List(labels.Everything())
	for _, p := range pdbs {
		snap.PodDisruptionBudgets = append(snap.PodDisruptionBudgets, MapPodDisruptionBudget(p))
	}
	serviceAccounts, _ := serviceAccountL.List(labels.Everything())
	for _, s := range serviceAccounts {
		snap.ServiceAccounts = append(snap.ServiceAccounts, MapServiceAccount(s))
	}
	roles, _ := roleL.List(labels.Everything())
	for _, r := range roles {
		snap.Roles = append(snap.Roles, MapRole(r))
	}
	clusterRoles, _ := clusterRoleL.List(labels.Everything())
	for _, r := range clusterRoles {
		snap.ClusterRoles = append(snap.ClusterRoles, MapClusterRole(r))
	}
	roleBindings, _ := roleBindingL.List(labels.Everything())
	for _, r := range roleBindings {
		snap.RoleBindings = append(snap.RoleBindings, MapRoleBinding(r))
	}
	clusterRoleBindings, _ := clusterRoleBindingL.List(labels.Everything())
	for _, r := range clusterRoleBindings {
		snap.ClusterRoleBindings = append(snap.ClusterRoleBindings, MapClusterRoleBinding(r))
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
