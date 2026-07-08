package state

// FluxView is a normalized snapshot of one Flux toolkit object
// (Kustomization, HelmRelease, GitRepository, …). Flux is a first-class
// citizen in kubagachi: these render in their own view with reconcile and
// suspend actions.
type FluxView struct {
	Kind      string // Kustomization | HelmRelease | GitRepository | OCIRepository | HelmRepository | Bucket
	Name      string
	Namespace string
	Ready     string // True | False | Unknown | "-"
	Suspended bool
	Revision  string
	Source    string   // e.g. "GitRepository/flux-system"
	DependsOn []string // ordering deps as "namespace/name" (spec.dependsOn)
	Message   string
	Age       string
}

// Key returns a stable unique identifier for the Flux object.
func (f FluxView) Key() string {
	return f.Kind + "/" + f.Namespace + "/" + f.Name
}

// HealthGlyph maps the Ready condition onto the same status vocabulary pods
// use, so flux objects can reuse status colors.
func (f FluxView) Health() string {
	if f.Suspended {
		return StatusBackOff
	}
	switch f.Ready {
	case "True":
		return StatusRunning
	case "False":
		return StatusFailed
	default:
		return StatusUnknown
	}
}
