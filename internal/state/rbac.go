package state

// NamespaceView is a normalized snapshot of a core/v1 Namespace.
type NamespaceView struct {
	Name       string
	Phase      string // Active | Terminating
	Labels     map[string]string
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the namespace.
func (n NamespaceView) Key() string { return n.Name }

// ServiceAccountView is a normalized snapshot of a core/v1 ServiceAccount.
type ServiceAccountView struct {
	Name             string
	Namespace        string
	Secrets          []string
	ImagePullSecrets []string
	AutomountToken   bool
	Age              string
	AgeSeconds       int64
}

// Key returns a stable unique identifier for the service account.
func (s ServiceAccountView) Key() string { return s.Namespace + "/" + s.Name }

// PolicyRuleView captures the parts of an RBAC policy rule shown in the web UI.
type PolicyRuleView struct {
	APIGroups []string
	Resources []string
	Verbs     []string
}

// RoleView is a normalized snapshot of an rbac.authorization.k8s.io/v1 Role.
type RoleView struct {
	Name       string
	Namespace  string
	Rules      []PolicyRuleView
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the role.
func (r RoleView) Key() string { return r.Namespace + "/" + r.Name }

// ClusterRoleView is a normalized snapshot of an rbac.authorization.k8s.io/v1 ClusterRole.
type ClusterRoleView struct {
	Name              string
	Rules             []PolicyRuleView
	AggregationLabels map[string]string
	Age               string
	AgeSeconds        int64
}

// Key returns a stable unique identifier for the cluster role.
func (r ClusterRoleView) Key() string { return r.Name }

// RoleRefView points a binding at the role it grants.
type RoleRefView struct {
	Kind string
	Name string
}

// SubjectView captures a RoleBinding or ClusterRoleBinding subject.
type SubjectView struct {
	Kind      string
	Name      string
	Namespace string
}

// RoleBindingView is a normalized snapshot of an rbac.authorization.k8s.io/v1 RoleBinding.
type RoleBindingView struct {
	Name       string
	Namespace  string
	RoleRef    RoleRefView
	Subjects   []SubjectView
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the role binding.
func (r RoleBindingView) Key() string { return r.Namespace + "/" + r.Name }

// ClusterRoleBindingView is a normalized snapshot of an rbac.authorization.k8s.io/v1 ClusterRoleBinding.
type ClusterRoleBindingView struct {
	Name       string
	RoleRef    RoleRefView
	Subjects   []SubjectView
	Age        string
	AgeSeconds int64
}

// Key returns a stable unique identifier for the cluster role binding.
func (r ClusterRoleBindingView) Key() string { return r.Name }

// CustomResourceDefinitionView is a normalized snapshot of an apiextensions.k8s.io/v1 CRD.
type CustomResourceDefinitionView struct {
	Name         string
	Group        string
	Scope        string // Namespaced | Cluster
	Versions     []string
	PluralName   string
	SingularName string
	ListKind     string
	ShortNames   []string
	Age          string
	AgeSeconds   int64
}

// Key returns a stable unique identifier for the custom resource definition.
func (c CustomResourceDefinitionView) Key() string { return c.Name }
