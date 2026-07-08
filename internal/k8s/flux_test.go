package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// TestMapFluxObjectDependsOn checks that spec.dependsOn is extracted as
// "namespace/name" edges, defaulting a missing namespace to the object's own,
// while the existing spec.sourceRef edge is still captured.
func TestMapFluxObjectDependsOn(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      "apps",
			"namespace": "flux-system",
		},
		"spec": map[string]any{
			"sourceRef": map[string]any{
				"kind": "GitRepository",
				"name": "platform",
			},
			"dependsOn": []any{
				// namespace omitted -> defaults to the object's namespace.
				map[string]any{"name": "infra"},
				// explicit namespace is preserved.
				map[string]any{"name": "secrets", "namespace": "vault"},
				// entries without a name are skipped.
				map[string]any{"namespace": "noise"},
			},
		},
	}}

	fv := mapFluxObject("Kustomization", u)

	if fv.Source != "GitRepository/platform" {
		t.Fatalf("source = %q, want GitRepository/platform", fv.Source)
	}
	want := []string{"flux-system/infra", "vault/secrets"}
	if len(fv.DependsOn) != len(want) {
		t.Fatalf("dependsOn = %v, want %v", fv.DependsOn, want)
	}
	for i, w := range want {
		if fv.DependsOn[i] != w {
			t.Fatalf("dependsOn[%d] = %q, want %q", i, fv.DependsOn[i], w)
		}
	}
}

// TestMapFluxObjectNoDependsOn ensures objects without spec.dependsOn map to a
// nil slice (so the json field is omitted on the wire).
func TestMapFluxObjectNoDependsOn(t *testing.T) {
	u := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "git", "namespace": "flux-system"},
	}}
	if fv := mapFluxObject("GitRepository", u); fv.DependsOn != nil {
		t.Fatalf("dependsOn = %v, want nil", fv.DependsOn)
	}
}
