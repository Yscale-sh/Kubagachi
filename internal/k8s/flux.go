package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/jakenesler/kubagachi/internal/state"
)

// fluxGVRs lists every Flux toolkit resource kubagachi treats as first-class.
// Order is the display order. Each entry may carry fallback API versions
// because Flux promoted groups at different times.
var fluxGVRs = []struct {
	kind     string
	versions []schema.GroupVersionResource
}{
	{"Kustomization", []schema.GroupVersionResource{
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Resource: "kustomizations"},
		{Group: "kustomize.toolkit.fluxcd.io", Version: "v1beta2", Resource: "kustomizations"},
	}},
	{"HelmRelease", []schema.GroupVersionResource{
		{Group: "helm.toolkit.fluxcd.io", Version: "v2", Resource: "helmreleases"},
		{Group: "helm.toolkit.fluxcd.io", Version: "v2beta2", Resource: "helmreleases"},
	}},
	{"GitRepository", []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "gitrepositories"},
	}},
	{"OCIRepository", []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "ocirepositories"},
		{Group: "source.toolkit.fluxcd.io", Version: "v1beta2", Resource: "ocirepositories"},
	}},
	{"HelmRepository", []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "helmrepositories"},
	}},
	{"Bucket", []schema.GroupVersionResource{
		{Group: "source.toolkit.fluxcd.io", Version: "v1", Resource: "buckets"},
	}},
}

// fluxResource is one resolved (kind, GVR) pair found on the cluster.
type fluxResource struct {
	kind string
	gvr  schema.GroupVersionResource
}

// discoverFlux probes the API server for the Flux CRDs and returns the
// resolved resources. An empty slice means Flux is not installed.
func discoverFlux(cs kubernetes.Interface) []fluxResource {
	var out []fluxResource
	disco := cs.Discovery()
	for _, entry := range fluxGVRs {
		for _, gvr := range entry.versions {
			list, err := disco.ServerResourcesForGroupVersion(gvr.GroupVersion().String())
			if err != nil {
				continue
			}
			for _, r := range list.APIResources {
				if r.Name == gvr.Resource {
					out = append(out, fluxResource{kind: entry.kind, gvr: gvr})
					break
				}
			}
			if len(out) > 0 && out[len(out)-1].kind == entry.kind {
				break // first matching version wins
			}
		}
	}
	return out
}

// listFlux lists every discovered Flux object in ns ("" = all namespaces) and
// maps it into FluxViews.
func listFlux(ctx context.Context, dyn dynamic.Interface, resources []fluxResource, ns string) []state.FluxView {
	var out []state.FluxView
	for _, fr := range resources {
		var iface dynamic.ResourceInterface = dyn.Resource(fr.gvr)
		if ns != "" {
			iface = dyn.Resource(fr.gvr).Namespace(ns)
		}
		list, err := iface.List(ctx, metav1.ListOptions{})
		if err != nil {
			continue
		}
		for i := range list.Items {
			out = append(out, mapFluxObject(fr.kind, &list.Items[i]))
		}
	}
	return out
}

// mapFluxObject converts an unstructured Flux object into a FluxView.
func mapFluxObject(kind string, u *unstructured.Unstructured) state.FluxView {
	fv := state.FluxView{
		Kind:      kind,
		Name:      u.GetName(),
		Namespace: u.GetNamespace(),
		Ready:     "-",
		Age:       humanizeAge(u.GetCreationTimestamp().Time),
	}
	if susp, found, _ := unstructured.NestedBool(u.Object, "spec", "suspend"); found {
		fv.Suspended = susp
	}

	conds, _, _ := unstructured.NestedSlice(u.Object, "status", "conditions")
	for _, c := range conds {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := cm["type"].(string); t == "Ready" {
			if s, _ := cm["status"].(string); s != "" {
				fv.Ready = s
			}
			fv.Message, _ = cm["message"].(string)
		}
	}

	for _, path := range [][]string{
		{"status", "lastAppliedRevision"},
		{"status", "artifact", "revision"},
		{"status", "lastAttemptedRevision"},
	} {
		if rev, found, _ := unstructured.NestedString(u.Object, path...); found && rev != "" {
			fv.Revision = shortRevision(rev)
			break
		}
	}

	// Source reference: Kustomization uses spec.sourceRef, HelmRelease nests
	// it under spec.chart.spec.sourceRef.
	for _, path := range [][]string{
		{"spec", "sourceRef"},
		{"spec", "chart", "spec", "sourceRef"},
	} {
		ref, found, _ := unstructured.NestedMap(u.Object, path...)
		if !found {
			continue
		}
		refKind, _ := ref["kind"].(string)
		refName, _ := ref["name"].(string)
		if refKind != "" && refName != "" {
			fv.Source = refKind + "/" + refName
			break
		}
	}

	// dependsOn ordering edges: Kustomization and HelmRelease both expose
	// spec.dependsOn as a list of {name, namespace} refs. A missing namespace
	// defaults to the object's own namespace. Stored as "namespace/name" so the
	// graph view can resolve each dependency back to a node.
	if deps, found, _ := unstructured.NestedSlice(u.Object, "spec", "dependsOn"); found {
		for _, d := range deps {
			dm, ok := d.(map[string]any)
			if !ok {
				continue
			}
			name, _ := dm["name"].(string)
			if name == "" {
				continue
			}
			ns, _ := dm["namespace"].(string)
			if ns == "" {
				ns = fv.Namespace
			}
			fv.DependsOn = append(fv.DependsOn, ns+"/"+name)
		}
	}
	return fv
}

// shortRevision compacts "main@sha1:abcdef0123456789…" style revisions.
func shortRevision(rev string) string {
	if i := strings.Index(rev, "sha1:"); i >= 0 {
		head := strings.TrimSuffix(rev[:i], "@")
		sha := rev[i+5:]
		if len(sha) > 8 {
			sha = sha[:8]
		}
		if head == "" {
			return sha
		}
		return head + "@" + sha
	}
	if len(rev) > 24 {
		return rev[:24] + "…"
	}
	return rev
}

// fluxGVRFor resolves a kubagachi flux kind back to its discovered GVR.
func fluxGVRFor(resources []fluxResource, kind string) (schema.GroupVersionResource, bool) {
	for _, fr := range resources {
		if strings.EqualFold(fr.kind, kind) {
			return fr.gvr, true
		}
	}
	return schema.GroupVersionResource{}, false
}

// FluxReconcile asks Flux to reconcile an object now by stamping the
// reconcile.fluxcd.io/requestedAt annotation, exactly like `flux reconcile`.
func (c *Client) FluxReconcile(ctx context.Context, kind, namespace, name string) error {
	resources := discoverFlux(c.Clientset)
	gvr, ok := fluxGVRFor(resources, kind)
	if !ok {
		return fmt.Errorf("flux kind %q not available on this cluster", kind)
	}
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				"reconcile.fluxcd.io/requestedAt": time.Now().Format(time.RFC3339Nano),
			},
		},
	}
	raw, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = c.Dynamic.Resource(gvr).Namespace(namespace).
		Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
	return err
}

// FluxSuspend toggles spec.suspend on a Flux object.
func (c *Client) FluxSuspend(ctx context.Context, kind, namespace, name string, suspend bool) error {
	resources := discoverFlux(c.Clientset)
	gvr, ok := fluxGVRFor(resources, kind)
	if !ok {
		return fmt.Errorf("flux kind %q not available on this cluster", kind)
	}
	raw, err := json.Marshal(map[string]any{"spec": map[string]any{"suspend": suspend}})
	if err != nil {
		return err
	}
	_, err = c.Dynamic.Resource(gvr).Namespace(namespace).
		Patch(ctx, name, types.MergePatchType, raw, metav1.PatchOptions{})
	return err
}
