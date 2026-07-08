package state

// Status constants represent the normalized critter/pod state used across
// the application. Kubernetes phases and container reasons are mapped onto
// these values by the k8s mapper.
const (
	StatusRunning     = "running"
	StatusPending     = "pending"
	StatusCompleted   = "completed"
	StatusFailed      = "failed"
	StatusUnknown     = "unknown"
	StatusCrashLoop   = "crashloop"
	StatusImagePull   = "imagepull"
	StatusBackOff     = "backoff"
	StatusOOMKilled   = "oomkilled"
	StatusTerminating = "terminating"
)

// ContainerView is a normalized snapshot of a single container in a pod.
type ContainerView struct {
	Name         string
	Image        string
	Ready        bool
	RestartCount int32
	State        string
	Reason       string
	Message      string
	ExitCode     int32
}

// PodView is a normalized snapshot of a pod, decoupled from the Kubernetes
// API types so the TUI never touches client-go objects directly.
type PodView struct {
	UID          string
	Name         string
	Namespace    string
	NodeName     string
	IP           string
	Phase        string
	Reason       string
	Status       string
	Restarts     int32
	Age          string
	AgeSeconds   int64
	Owner        string
	Containers   []ContainerView
	Critter      string
	CritterState string

	CPUUsedMilli int64 // -1 == unknown
	MemUsedBytes int64 // -1 == unknown
}

// Key returns a stable unique identifier for the pod.
func (p PodView) Key() string {
	return p.Namespace + "/" + p.Name
}

// ReadyText returns an "n/m" string of ready vs total containers.
func (p PodView) ReadyText() string {
	ready := 0
	for _, c := range p.Containers {
		if c.Ready {
			ready++
		}
	}
	return itoa(ready) + "/" + itoa(len(p.Containers))
}
