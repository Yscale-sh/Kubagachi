package k8s

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	sigsyaml "sigs.k8s.io/yaml"
)

// SecretValueRaw holds the base64-encoded and decoded value of a single
// secret key. Defined here (not in internal/tui) to keep client-go imports
// out of the tui package. The adapter in internal/app/source.go converts it
// to tui.SecretValue.
type SecretValueRaw struct {
	B64     string
	Decoded string
}

// CustomResourceRefRaw is a compact reference to a custom resource instance.
// The adapter in internal/app/source.go converts it to tui.CustomResourceRef.
type CustomResourceRefRaw struct {
	Name      string
	Namespace string
	AgeSec    int64
}

// builtinKindGroupVersion maps the resource kinds the web UI can show to their
// apiGroup/version. The UI's normalized resources do not carry an apiVersion, so
// ObjectYAML falls back to this table when the caller omits it. Without it a bare
// GroupKind{Group:""} only resolves core/v1 kinds and every non-core kind (RBAC,
// CRDs, apps, networking, autoscaling, policy, storage) fails to map to a GVR.
var builtinKindGroupVersion = map[string]schema.GroupVersion{
	// core/v1
	"Pod":                   {Group: "", Version: "v1"},
	"Service":               {Group: "", Version: "v1"},
	"ConfigMap":             {Group: "", Version: "v1"},
	"Secret":                {Group: "", Version: "v1"},
	"Endpoints":             {Group: "", Version: "v1"},
	"Namespace":             {Group: "", Version: "v1"},
	"ServiceAccount":        {Group: "", Version: "v1"},
	"ResourceQuota":         {Group: "", Version: "v1"},
	"LimitRange":            {Group: "", Version: "v1"},
	"Node":                  {Group: "", Version: "v1"},
	"Event":                 {Group: "", Version: "v1"},
	"PersistentVolume":      {Group: "", Version: "v1"},
	"PersistentVolumeClaim": {Group: "", Version: "v1"},
	// apps/v1
	"Deployment":  {Group: "apps", Version: "v1"},
	"StatefulSet": {Group: "apps", Version: "v1"},
	"DaemonSet":   {Group: "apps", Version: "v1"},
	"ReplicaSet":  {Group: "apps", Version: "v1"},
	// batch/v1
	"Job":     {Group: "batch", Version: "v1"},
	"CronJob": {Group: "batch", Version: "v1"},
	// networking / autoscaling / policy / storage
	"Ingress":                 {Group: "networking.k8s.io", Version: "v1"},
	"NetworkPolicy":           {Group: "networking.k8s.io", Version: "v1"},
	"HorizontalPodAutoscaler": {Group: "autoscaling", Version: "v2"},
	"PodDisruptionBudget":     {Group: "policy", Version: "v1"},
	"StorageClass":            {Group: "storage.k8s.io", Version: "v1"},
	// rbac
	"Role":               {Group: "rbac.authorization.k8s.io", Version: "v1"},
	"ClusterRole":        {Group: "rbac.authorization.k8s.io", Version: "v1"},
	"RoleBinding":        {Group: "rbac.authorization.k8s.io", Version: "v1"},
	"ClusterRoleBinding": {Group: "rbac.authorization.k8s.io", Version: "v1"},
	// apiextensions
	"CustomResourceDefinition": {Group: "apiextensions.k8s.io", Version: "v1"},
}

type resolvedResource struct {
	resource   schema.GroupVersionResource
	namespaced bool
}

func (c *Client) resolveResource(apiVersion, kind string) (resolvedResource, error) {
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(
		memory.NewMemCacheClient(c.Clientset.Discovery()),
	)

	var group, version string
	if apiVersion != "" {
		if idx := strings.Index(apiVersion, "/"); idx != -1 {
			group = apiVersion[:idx]
			version = apiVersion[idx+1:]
		} else {
			// bare version like "v1" — core group
			version = apiVersion
		}
	} else if gv, ok := builtinKindGroupVersion[kind]; ok {
		// No apiVersion from the caller — resolve the kind's group/version from the
		// built-in table so non-core kinds (RBAC, CRDs, apps, …) map correctly.
		group, version = gv.Group, gv.Version
	}

	gk := schema.GroupKind{Group: group, Kind: kind}
	var versions []string
	if version != "" {
		versions = []string{version}
	}
	mapping, err := mapper.RESTMapping(gk, versions...)
	if err != nil {
		return resolvedResource{}, fmt.Errorf("resolving resource for apiVersion=%q kind=%q: %w", apiVersion, kind, err)
	}
	return resolvedResource{
		resource:   mapping.Resource,
		namespaced: mapping.Scope.Name() == "namespace",
	}, nil
}

func (c *Client) actionNamespace(namespace string) string {
	if namespace != "" {
		return namespace
	}
	if c.DefaultNamespace != "" {
		return c.DefaultNamespace
	}
	return "default"
}

// ObjectYAML fetches the object identified by apiVersion/kind/namespace/name,
// strips noisy metadata fields (managedFields and the
// kubectl.kubernetes.io/last-applied-configuration annotation), and returns
// the object serialized as YAML.
//
// apiVersion may be "" (preferred version resolved via discovery), "v1" (core
// group), or "group/version" (e.g. "apps/v1"). namespace must be empty for
// cluster-scoped resources.
func (c *Client) ObjectYAML(ctx context.Context, apiVersion, kind, namespace, name string) (string, error) {
	mapping, err := c.resolveResource(apiVersion, kind)
	if err != nil {
		return "", err
	}

	// Fetch the object. The dynamic client returns *unstructured.Unstructured;
	// we access its .Object map directly without importing the package.
	var obj map[string]interface{}
	if mapping.namespaced {
		u, err := c.Dynamic.Resource(mapping.resource).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("get %s/%s in namespace %q: %w", kind, name, namespace, err)
		}
		obj = u.Object
	} else {
		u, err := c.Dynamic.Resource(mapping.resource).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("get %s/%s: %w", kind, name, err)
		}
		obj = u.Object
	}

	// Strip noisy / large metadata fields.
	if md, ok := obj["metadata"].(map[string]interface{}); ok {
		delete(md, "managedFields")
		if ann, ok := md["annotations"].(map[string]interface{}); ok {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		}
	}

	out, err := sigsyaml.Marshal(obj)
	if err != nil {
		return "", fmt.Errorf("marshaling object as YAML: %w", err)
	}
	return string(out), nil
}

// DeleteResource deletes any Kubernetes resource resolvable through discovery.
func (c *Client) DeleteResource(ctx context.Context, apiVersion, kind, namespace, name string) error {
	mapping, err := c.resolveResource(apiVersion, kind)
	if err != nil {
		return err
	}
	if mapping.namespaced {
		ns := c.actionNamespace(namespace)
		if err := c.Dynamic.Resource(mapping.resource).Namespace(ns).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("delete %s/%s in namespace %q: %w", kind, name, ns, err)
		}
		return nil
	}
	if err := c.Dynamic.Resource(mapping.resource).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return fmt.Errorf("delete %s/%s: %w", kind, name, err)
	}
	return nil
}

func scalableKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "ReplicaSet":
		return true
	default:
		return false
	}
}

// ScaleResource patches spec.replicas for scalable workload resources.
func (c *Client) ScaleResource(ctx context.Context, apiVersion, kind, namespace, name string, replicas int32) error {
	if !scalableKind(kind) {
		return fmt.Errorf("scale is only supported for Deployment, StatefulSet, and ReplicaSet")
	}
	if replicas < 0 {
		return fmt.Errorf("replicas must be non-negative")
	}
	mapping, err := c.resolveResource(apiVersion, kind)
	if err != nil {
		return err
	}
	patch := []byte(fmt.Sprintf(`{"spec":{"replicas":%d}}`, replicas))
	if mapping.namespaced {
		ns := c.actionNamespace(namespace)
		if _, err := c.Dynamic.Resource(mapping.resource).Namespace(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("scale %s/%s in namespace %q: %w", kind, name, ns, err)
		}
		return nil
	}
	if _, err := c.Dynamic.Resource(mapping.resource).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("scale %s/%s: %w", kind, name, err)
	}
	return nil
}

func restartableKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return true
	default:
		return false
	}
}

// RestartResource performs a kubectl-rollout-restart-style pod template patch.
func (c *Client) RestartResource(ctx context.Context, apiVersion, kind, namespace, name string) error {
	if !restartableKind(kind) {
		return fmt.Errorf("restart is only supported for Deployment, StatefulSet, and DaemonSet")
	}
	mapping, err := c.resolveResource(apiVersion, kind)
	if err != nil {
		return err
	}
	restartedAt := time.Now().UTC().Format(time.RFC3339)
	patch := []byte(fmt.Sprintf(`{"spec":{"template":{"metadata":{"annotations":{"kubagachi.io/restartedAt":%q}}}}}`, restartedAt))
	if mapping.namespaced {
		ns := c.actionNamespace(namespace)
		if _, err := c.Dynamic.Resource(mapping.resource).Namespace(ns).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return fmt.Errorf("restart %s/%s in namespace %q: %w", kind, name, ns, err)
		}
		return nil
	}
	if _, err := c.Dynamic.Resource(mapping.resource).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("restart %s/%s: %w", kind, name, err)
	}
	return nil
}

// CordonNode patches a Node's spec.unschedulable field. cordon=true marks the
// node unschedulable; cordon=false clears it. The GVR is built directly — no
// RESTMapper is needed for the well-known core/v1 Node resource.
func (c *Client) CordonNode(ctx context.Context, name string, cordon bool) error {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	unschedulable := "false"
	if cordon {
		unschedulable = "true"
	}
	patch := []byte(`{"spec":{"unschedulable":` + unschedulable + `}}`)
	if _, err := c.Dynamic.Resource(gvr).Patch(ctx, name, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
		return fmt.Errorf("cordon node %s (cordon=%v): %w", name, cordon, err)
	}
	return nil
}

// CustomResources lists instances for an already-known custom resource GVR.
func (c *Client) CustomResources(ctx context.Context, group, version, resource, namespace string) ([]CustomResourceRefRaw, error) {
	gvr := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: resource,
	}

	var list *unstructured.UnstructuredList
	var err error
	if namespace != "" {
		list, err = c.Dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	} else {
		list, err = c.Dynamic.Resource(gvr).List(ctx, metav1.ListOptions{})
	}
	if err != nil {
		return nil, fmt.Errorf("list custom resources %s/%s/%s namespace %q: %w", group, version, resource, namespace, err)
	}

	out := make([]CustomResourceRefRaw, 0, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		var ageSec int64
		created := item.GetCreationTimestamp()
		if !created.IsZero() {
			ageSec = int64(time.Since(created.Time).Seconds())
			if ageSec < 0 {
				ageSec = 0
			}
		}
		out = append(out, CustomResourceRefRaw{
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			AgeSec:    ageSec,
		})
	}
	return out, nil
}

// ApplyYAML server-side-applies the given YAML document to the cluster.
// It reads apiVersion/kind/namespace/name from the document itself, resolves
// the GVR via the same discovery-backed mapper used in ObjectYAML, then calls
// dynamic.Resource(gvr).Namespace(ns).Apply with FieldManager "kubagachi"
// and Force:true. The applied object (minus managedFields) is returned as YAML.
func (c *Client) ApplyYAML(ctx context.Context, yaml string) (string, error) {
	// Decode the YAML into an unstructured map.
	var obj map[string]interface{}
	if err := sigsyaml.Unmarshal([]byte(yaml), &obj); err != nil {
		return "", fmt.Errorf("parsing YAML: %w", err)
	}
	if len(obj) == 0 {
		return "", fmt.Errorf("empty YAML document")
	}

	u := &unstructured.Unstructured{Object: obj}
	apiVersion := u.GetAPIVersion()
	kind := u.GetKind()
	name := u.GetName()
	namespace := u.GetNamespace()

	if kind == "" || name == "" {
		return "", fmt.Errorf("YAML must contain kind and metadata.name")
	}

	mapping, err := c.resolveResource(apiVersion, kind)
	if err != nil {
		return "", err
	}

	opts := metav1.ApplyOptions{FieldManager: "kubagachi", Force: true}

	var result *unstructured.Unstructured
	if mapping.namespaced {
		ns := namespace
		if ns == "" {
			ns = "default"
		}
		result, err = c.Dynamic.Resource(mapping.resource).Namespace(ns).Apply(ctx, name, u, opts)
	} else {
		result, err = c.Dynamic.Resource(mapping.resource).Apply(ctx, name, u, opts)
	}
	if err != nil {
		return "", fmt.Errorf("applying %s/%s: %w", kind, name, err)
	}

	// Strip noisy / large metadata fields before returning.
	if md, ok := result.Object["metadata"].(map[string]interface{}); ok {
		delete(md, "managedFields")
		if ann, ok := md["annotations"].(map[string]interface{}); ok {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
		}
	}

	out, err := sigsyaml.Marshal(result.Object)
	if err != nil {
		return "", fmt.Errorf("marshaling applied object as YAML: %w", err)
	}
	return string(out), nil
}

// SecretData fetches the named Secret and returns each key's raw bytes as
// base64 and decoded UTF-8. The caller (internal/app/source.go) converts the
// result to the tui-layer type.
func (c *Client) SecretData(ctx context.Context, namespace, name string) (map[string]SecretValueRaw, error) {
	secret, err := c.Clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", namespace, name, err)
	}
	out := make(map[string]SecretValueRaw, len(secret.Data))
	for k, v := range secret.Data {
		out[k] = SecretValueRaw{
			B64:     base64.StdEncoding.EncodeToString(v),
			Decoded: string(v),
		}
	}
	return out, nil
}
