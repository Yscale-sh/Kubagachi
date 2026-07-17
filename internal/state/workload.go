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

// StatefulSetView is a normalized snapshot of an apps/v1 StatefulSet.
type StatefulSetView struct {
	Name          string
	Namespace     string
	Replicas      int32
	ReadyReplicas int32
	ServiceName   string
	Image         string
	Age           string
	AgeSeconds    int64
}

// Key returns a stable unique identifier for the stateful set.
func (s StatefulSetView) Key() string { return s.Namespace + "/" + s.Name }

// DaemonSetView is a normalized snapshot of an apps/v1 DaemonSet.
type DaemonSetView struct {
	Name                   string
	Namespace              string
	DesiredNumberScheduled int32
	NumberReady            int32
	NumberAvailable        int32
	Image                  string
	Age                    string
	AgeSeconds             int64
}

// Key returns a stable unique identifier for the daemon set.
func (d DaemonSetView) Key() string { return d.Namespace + "/" + d.Name }

// ReplicaSetView is a normalized snapshot of an apps/v1 ReplicaSet.
type ReplicaSetView struct {
	Name          string
	Namespace     string
	Replicas      int32
	ReadyReplicas int32
	OwnerKind     string
	OwnerName     string
	Image         string
	Age           string
	AgeSeconds    int64
}

// Key returns a stable unique identifier for the replica set.
func (r ReplicaSetView) Key() string { return r.Namespace + "/" + r.Name }

// JobView is a normalized snapshot of a batch/v1 Job.
type JobView struct {
	Name        string
	Namespace   string
	Completions int32
	Succeeded   int32
	Failed      int32
	Active      int32
	Status      string
	Image       string
	DurationSec int64
	Age         string
	AgeSeconds  int64
	// OwnerKind/OwnerName carry the controller that created this Job — a CronJob
	// for scheduled runs — so the UI can tell a superseded old run from the
	// current one when deriving cluster health.
	OwnerKind string
	OwnerName string
}

// Key returns a stable unique identifier for the job.
func (j JobView) Key() string { return j.Namespace + "/" + j.Name }

// CronJobView is a normalized snapshot of a batch/v1 CronJob.
type CronJobView struct {
	Name               string
	Namespace          string
	Schedule           string
	Suspend            bool
	LastScheduleAgeSec int64
	HasLastSchedule    bool
	ActiveJobs         int
	Status             string
	Image              string
	Age                string
	AgeSeconds         int64
}

// Key returns a stable unique identifier for the cron job.
func (c CronJobView) Key() string { return c.Namespace + "/" + c.Name }

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

// IngressRuleView captures a single host/path backend route.
type IngressRuleView struct {
	Host        string
	Path        string
	ServiceName string
	ServicePort int32
}

// IngressView is a normalized snapshot of a networking.k8s.io/v1 Ingress.
type IngressView struct {
	Name       string
	Namespace  string
	ClassName  string
	Hosts      []string
	Rules      []IngressRuleView
	TLS        bool
	Address    string
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the ingress.
func (i IngressView) Key() string { return i.Namespace + "/" + i.Name }

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

// SecretView is a metadata-only normalized snapshot of a core/v1 Secret.
type SecretView struct {
	Name       string
	Namespace  string
	Type       string
	Keys       []string
	DataBytes  int
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the secret.
func (s SecretView) Key() string { return s.Namespace + "/" + s.Name }

// PersistentVolumeClaimView is a normalized snapshot of a core/v1 PVC.
type PersistentVolumeClaimView struct {
	Name             string
	Namespace        string
	Capacity         string
	AccessModes      []string
	StorageClassName string
	Phase            string
	VolumeName       string
	Age              string
	AgeSeconds       int64
}

// Key returns a stable unique identifier for the persistent volume claim.
func (p PersistentVolumeClaimView) Key() string { return p.Namespace + "/" + p.Name }

// PersistentVolumeView is a normalized snapshot of a core/v1 PV.
type PersistentVolumeView struct {
	Name             string
	Capacity         string
	AccessModes      []string
	ReclaimPolicy    string
	Phase            string
	StorageClassName string
	ClaimNamespace   string
	ClaimName        string
	Age              string
	AgeSeconds       int64
}

// Key returns a stable unique identifier for the persistent volume.
func (p PersistentVolumeView) Key() string { return p.Name }

// StorageClassView is a normalized snapshot of a storage.k8s.io/v1 StorageClass.
type StorageClassView struct {
	Name              string
	Provisioner       string
	ReclaimPolicy     string
	VolumeBindingMode string
	IsDefault         bool
	Age               string
	AgeSeconds        int64
}

// Key returns a stable unique identifier for the storage class.
func (s StorageClassView) Key() string { return s.Name }
