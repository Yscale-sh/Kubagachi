package state

// DeploymentView is a normalized snapshot of an apps/v1 Deployment.
type DeploymentView struct {
	Name       string
	Namespace  string
	Replicas   int32  // spec.replicas (desired)
	Ready      int32  // status.readyReplicas
	Updated    int32  // status.updatedReplicas
	Available  int32  // status.availableReplicas
	Image      string // first container image in the pod template
	Selector   string // "k=v,k=v" (sorted keys)
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the deployment.
func (d DeploymentView) Key() string { return d.Namespace + "/" + d.Name }

// ServicePortView captures a single port exposed by a Service.
type ServicePortView struct {
	Name       string
	Port       int32
	TargetPort int32
	NodePort   int32
	Protocol   string // TCP | UDP
}

// ServiceView is a normalized snapshot of a core/v1 Service.
type ServiceView struct {
	Name       string
	Namespace  string
	Type       string // ClusterIP | NodePort | LoadBalancer | ExternalName | Headless
	ClusterIP  string
	ExternalIP string
	Ports      []ServicePortView
	Selector   string // "k=v,k=v" (sorted keys)
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the service.
func (s ServiceView) Key() string { return s.Namespace + "/" + s.Name }

// ConfigMapView is a normalized snapshot of a core/v1 ConfigMap.
type ConfigMapView struct {
	Name       string
	Namespace  string
	Keys       []string // data key names (sorted)
	DataBytes  int      // total bytes across all data values
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the config map.
func (c ConfigMapView) Key() string { return c.Namespace + "/" + c.Name }
