package state

import "sort"

// SummaryView holds aggregate counts shown in the header bar.
type SummaryView struct {
	Nodes     int
	Pods      int
	Running   int
	Pending   int
	CrashLoop int
	BackOff   int
	Failed    int
	Unknown   int
}

// HelmReleaseView is the normalized view of a Helm release (latest revision).
type HelmReleaseView struct {
	Name          string
	Namespace     string
	Chart         string
	ChartVersion  string
	AppVersion    string
	Revision      int
	Status        string
	UpdatedAgeSec int64
	AgeSeconds    int64
}

// ClusterState is the single normalized snapshot the TUI renders. Every
// refresh (real or demo) produces a fresh ClusterState.
type ClusterState struct {
	ClusterName               string
	ServerVersion             string
	Namespace                 string
	AllNamespaces             bool
	Nodes                     []NodeView
	Namespaces                []NamespaceView
	Pods                      []PodView
	Events                    []EventView
	Flux                      []FluxView
	Deployments               []DeploymentView
	StatefulSets              []StatefulSetView
	DaemonSets                []DaemonSetView
	ReplicaSets               []ReplicaSetView
	Jobs                      []JobView
	CronJobs                  []CronJobView
	Services                  []ServiceView
	Ingresses                 []IngressView
	Endpoints                 []EndpointView
	NetworkPolicies           []NetworkPolicyView
	ConfigMaps                []ConfigMapView
	Secrets                   []SecretView
	ResourceQuotas            []ResourceQuotaView
	LimitRanges               []LimitRangeView
	HorizontalPodAutoscalers  []HorizontalPodAutoscalerView
	PodDisruptionBudgets      []PodDisruptionBudgetView
	ServiceAccounts           []ServiceAccountView
	Roles                     []RoleView
	ClusterRoles              []ClusterRoleView
	RoleBindings              []RoleBindingView
	ClusterRoleBindings       []ClusterRoleBindingView
	CustomResourceDefinitions []CustomResourceDefinitionView
	PersistentVolumeClaims    []PersistentVolumeClaimView
	PersistentVolumes         []PersistentVolumeView
	StorageClasses            []StorageClassView
	HelmReleases              []HelmReleaseView
	FluxInstalled             bool
	MetricsInstalled          bool
	Summary                   SummaryView
}

// Rebuild recomputes derived data: it groups pods under their nodes, sorts
// everything deterministically, and recalculates the summary counts. Call
// this after mutating Nodes/Pods/Events so the snapshot is self-consistent.
func (c *ClusterState) Rebuild() {
	sort.Slice(c.Pods, func(i, j int) bool {
		if c.Pods[i].Namespace != c.Pods[j].Namespace {
			return c.Pods[i].Namespace < c.Pods[j].Namespace
		}
		return c.Pods[i].Name < c.Pods[j].Name
	})
	sort.Slice(c.Nodes, func(i, j int) bool {
		return c.Nodes[i].Name < c.Nodes[j].Name
	})
	sort.Slice(c.Namespaces, func(i, j int) bool {
		return c.Namespaces[i].Name < c.Namespaces[j].Name
	})
	sort.Slice(c.Flux, func(i, j int) bool {
		if c.Flux[i].Kind != c.Flux[j].Kind {
			return c.Flux[i].Kind < c.Flux[j].Kind
		}
		if c.Flux[i].Namespace != c.Flux[j].Namespace {
			return c.Flux[i].Namespace < c.Flux[j].Namespace
		}
		return c.Flux[i].Name < c.Flux[j].Name
	})

	sort.Slice(c.Deployments, func(i, j int) bool {
		if c.Deployments[i].Namespace != c.Deployments[j].Namespace {
			return c.Deployments[i].Namespace < c.Deployments[j].Namespace
		}
		return c.Deployments[i].Name < c.Deployments[j].Name
	})
	sort.Slice(c.StatefulSets, func(i, j int) bool {
		if c.StatefulSets[i].Namespace != c.StatefulSets[j].Namespace {
			return c.StatefulSets[i].Namespace < c.StatefulSets[j].Namespace
		}
		return c.StatefulSets[i].Name < c.StatefulSets[j].Name
	})
	sort.Slice(c.DaemonSets, func(i, j int) bool {
		if c.DaemonSets[i].Namespace != c.DaemonSets[j].Namespace {
			return c.DaemonSets[i].Namespace < c.DaemonSets[j].Namespace
		}
		return c.DaemonSets[i].Name < c.DaemonSets[j].Name
	})
	sort.Slice(c.ReplicaSets, func(i, j int) bool {
		if c.ReplicaSets[i].Namespace != c.ReplicaSets[j].Namespace {
			return c.ReplicaSets[i].Namespace < c.ReplicaSets[j].Namespace
		}
		return c.ReplicaSets[i].Name < c.ReplicaSets[j].Name
	})
	sort.Slice(c.Jobs, func(i, j int) bool {
		if c.Jobs[i].Namespace != c.Jobs[j].Namespace {
			return c.Jobs[i].Namespace < c.Jobs[j].Namespace
		}
		return c.Jobs[i].Name < c.Jobs[j].Name
	})
	sort.Slice(c.CronJobs, func(i, j int) bool {
		if c.CronJobs[i].Namespace != c.CronJobs[j].Namespace {
			return c.CronJobs[i].Namespace < c.CronJobs[j].Namespace
		}
		return c.CronJobs[i].Name < c.CronJobs[j].Name
	})
	sort.Slice(c.Services, func(i, j int) bool {
		if c.Services[i].Namespace != c.Services[j].Namespace {
			return c.Services[i].Namespace < c.Services[j].Namespace
		}
		return c.Services[i].Name < c.Services[j].Name
	})
	sort.Slice(c.Ingresses, func(i, j int) bool {
		if c.Ingresses[i].Namespace != c.Ingresses[j].Namespace {
			return c.Ingresses[i].Namespace < c.Ingresses[j].Namespace
		}
		return c.Ingresses[i].Name < c.Ingresses[j].Name
	})
	sort.Slice(c.Endpoints, func(i, j int) bool {
		if c.Endpoints[i].Namespace != c.Endpoints[j].Namespace {
			return c.Endpoints[i].Namespace < c.Endpoints[j].Namespace
		}
		return c.Endpoints[i].Name < c.Endpoints[j].Name
	})
	sort.Slice(c.NetworkPolicies, func(i, j int) bool {
		if c.NetworkPolicies[i].Namespace != c.NetworkPolicies[j].Namespace {
			return c.NetworkPolicies[i].Namespace < c.NetworkPolicies[j].Namespace
		}
		return c.NetworkPolicies[i].Name < c.NetworkPolicies[j].Name
	})
	sort.Slice(c.ConfigMaps, func(i, j int) bool {
		if c.ConfigMaps[i].Namespace != c.ConfigMaps[j].Namespace {
			return c.ConfigMaps[i].Namespace < c.ConfigMaps[j].Namespace
		}
		return c.ConfigMaps[i].Name < c.ConfigMaps[j].Name
	})
	sort.Slice(c.Secrets, func(i, j int) bool {
		if c.Secrets[i].Namespace != c.Secrets[j].Namespace {
			return c.Secrets[i].Namespace < c.Secrets[j].Namespace
		}
		return c.Secrets[i].Name < c.Secrets[j].Name
	})
	sort.Slice(c.ResourceQuotas, func(i, j int) bool {
		if c.ResourceQuotas[i].Namespace != c.ResourceQuotas[j].Namespace {
			return c.ResourceQuotas[i].Namespace < c.ResourceQuotas[j].Namespace
		}
		return c.ResourceQuotas[i].Name < c.ResourceQuotas[j].Name
	})
	sort.Slice(c.LimitRanges, func(i, j int) bool {
		if c.LimitRanges[i].Namespace != c.LimitRanges[j].Namespace {
			return c.LimitRanges[i].Namespace < c.LimitRanges[j].Namespace
		}
		return c.LimitRanges[i].Name < c.LimitRanges[j].Name
	})
	sort.Slice(c.HorizontalPodAutoscalers, func(i, j int) bool {
		if c.HorizontalPodAutoscalers[i].Namespace != c.HorizontalPodAutoscalers[j].Namespace {
			return c.HorizontalPodAutoscalers[i].Namespace < c.HorizontalPodAutoscalers[j].Namespace
		}
		return c.HorizontalPodAutoscalers[i].Name < c.HorizontalPodAutoscalers[j].Name
	})
	sort.Slice(c.PodDisruptionBudgets, func(i, j int) bool {
		if c.PodDisruptionBudgets[i].Namespace != c.PodDisruptionBudgets[j].Namespace {
			return c.PodDisruptionBudgets[i].Namespace < c.PodDisruptionBudgets[j].Namespace
		}
		return c.PodDisruptionBudgets[i].Name < c.PodDisruptionBudgets[j].Name
	})
	sort.Slice(c.ServiceAccounts, func(i, j int) bool {
		if c.ServiceAccounts[i].Namespace != c.ServiceAccounts[j].Namespace {
			return c.ServiceAccounts[i].Namespace < c.ServiceAccounts[j].Namespace
		}
		return c.ServiceAccounts[i].Name < c.ServiceAccounts[j].Name
	})
	sort.Slice(c.Roles, func(i, j int) bool {
		if c.Roles[i].Namespace != c.Roles[j].Namespace {
			return c.Roles[i].Namespace < c.Roles[j].Namespace
		}
		return c.Roles[i].Name < c.Roles[j].Name
	})
	sort.Slice(c.ClusterRoles, func(i, j int) bool {
		return c.ClusterRoles[i].Name < c.ClusterRoles[j].Name
	})
	sort.Slice(c.RoleBindings, func(i, j int) bool {
		if c.RoleBindings[i].Namespace != c.RoleBindings[j].Namespace {
			return c.RoleBindings[i].Namespace < c.RoleBindings[j].Namespace
		}
		return c.RoleBindings[i].Name < c.RoleBindings[j].Name
	})
	sort.Slice(c.ClusterRoleBindings, func(i, j int) bool {
		return c.ClusterRoleBindings[i].Name < c.ClusterRoleBindings[j].Name
	})
	sort.Slice(c.CustomResourceDefinitions, func(i, j int) bool {
		return c.CustomResourceDefinitions[i].Name < c.CustomResourceDefinitions[j].Name
	})
	sort.Slice(c.PersistentVolumeClaims, func(i, j int) bool {
		if c.PersistentVolumeClaims[i].Namespace != c.PersistentVolumeClaims[j].Namespace {
			return c.PersistentVolumeClaims[i].Namespace < c.PersistentVolumeClaims[j].Namespace
		}
		return c.PersistentVolumeClaims[i].Name < c.PersistentVolumeClaims[j].Name
	})
	sort.Slice(c.PersistentVolumes, func(i, j int) bool {
		return c.PersistentVolumes[i].Name < c.PersistentVolumes[j].Name
	})
	sort.Slice(c.StorageClasses, func(i, j int) bool {
		return c.StorageClasses[i].Name < c.StorageClasses[j].Name
	})
	sort.Slice(c.HelmReleases, func(i, j int) bool {
		if c.HelmReleases[i].Namespace != c.HelmReleases[j].Namespace {
			return c.HelmReleases[i].Namespace < c.HelmReleases[j].Namespace
		}
		return c.HelmReleases[i].Name < c.HelmReleases[j].Name
	})

	byNode := map[string][]PodView{}
	for _, p := range c.Pods {
		byNode[p.NodeName] = append(byNode[p.NodeName], p)
	}
	for i := range c.Nodes {
		c.Nodes[i].Pods = byNode[c.Nodes[i].Name]
	}

	c.Summary = SummaryView{Nodes: len(c.Nodes), Pods: len(c.Pods)}
	for _, p := range c.Pods {
		switch p.Status {
		case StatusRunning:
			c.Summary.Running++
		case StatusPending:
			c.Summary.Pending++
		case StatusCrashLoop, StatusOOMKilled:
			c.Summary.CrashLoop++
		case StatusBackOff, StatusImagePull:
			c.Summary.BackOff++
		case StatusFailed:
			c.Summary.Failed++
		case StatusUnknown:
			c.Summary.Unknown++
		}
	}
}

// FlatPods returns every pod in node-grouped order, which is the order the
// habitat view renders and the order keyboard navigation walks.
func (c *ClusterState) FlatPods() []PodView {
	out := make([]PodView, 0, len(c.Pods))
	for _, n := range c.Nodes {
		out = append(out, n.Pods...)
	}
	// Pods on unknown/empty nodes still need to appear.
	known := map[string]bool{}
	for _, n := range c.Nodes {
		known[n.Name] = true
	}
	for _, p := range c.Pods {
		if !known[p.NodeName] {
			out = append(out, p)
		}
	}
	return out
}
