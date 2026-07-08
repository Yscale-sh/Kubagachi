package state

// EndpointSubsetView captures the addresses and ports in one Endpoints subset.
type EndpointSubsetView struct {
	Addresses []string
	Ports     []int
}

// EndpointView is a normalized snapshot of a core/v1 Endpoints object.
type EndpointView struct {
	Name          string
	Namespace     string
	Subsets       []EndpointSubsetView
	TargetService string
	Age           string
	AgeSeconds    int64
}

// Key returns a stable unique identifier for the endpoint.
func (e EndpointView) Key() string { return e.Namespace + "/" + e.Name }

// NetworkPolicyView is a normalized snapshot of a networking.k8s.io/v1 NetworkPolicy.
type NetworkPolicyView struct {
	Name         string
	Namespace    string
	PodSelector  map[string]string
	PolicyTypes  []string
	IngressRules int
	EgressRules  int
	Age          string
	AgeSeconds   int64
}

// Key returns a stable unique identifier for the network policy.
func (n NetworkPolicyView) Key() string { return n.Namespace + "/" + n.Name }

// ResourceQuotaView is a normalized snapshot of a core/v1 ResourceQuota.
type ResourceQuotaView struct {
	Name       string
	Namespace  string
	Hard       map[string]string
	Used       map[string]string
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the resource quota.
func (r ResourceQuotaView) Key() string { return r.Namespace + "/" + r.Name }

// LimitRangeItemView captures default request/limit values for one LimitRange item.
type LimitRangeItemView struct {
	Type           string
	DefaultRequest map[string]string
	DefaultLimit   map[string]string
}

// LimitRangeView is a normalized snapshot of a core/v1 LimitRange.
type LimitRangeView struct {
	Name       string
	Namespace  string
	Limits     []LimitRangeItemView
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the limit range.
func (l LimitRangeView) Key() string { return l.Namespace + "/" + l.Name }

// HorizontalPodAutoscalerView is a normalized snapshot of an autoscaling/v2 HPA.
type HorizontalPodAutoscalerView struct {
	Name                 string
	Namespace            string
	TargetKind           string
	TargetName           string
	MinReplicas          int32
	MaxReplicas          int32
	CurrentReplicas      int32
	TargetCPUPercent     int32
	CurrentCPUPercent    int32
	HasTargetCPUPercent  bool
	HasCurrentCPUPercent bool
	Age                  string
	AgeSeconds           int64
}

// Key returns a stable unique identifier for the horizontal pod autoscaler.
func (h HorizontalPodAutoscalerView) Key() string { return h.Namespace + "/" + h.Name }

// PodDisruptionBudgetView is a normalized snapshot of a policy/v1 PDB.
type PodDisruptionBudgetView struct {
	Name           string
	Namespace      string
	MinAvailable   string
	MaxUnavailable string
	CurrentHealthy int32
	DesiredHealthy int32
	ExpectedPods   int32
	Selector       map[string]string
	Age            string
	AgeSeconds     int64
}

// Key returns a stable unique identifier for the pod disruption budget.
func (p PodDisruptionBudgetView) Key() string { return p.Namespace + "/" + p.Name }
