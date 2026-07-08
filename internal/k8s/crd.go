package k8s

import (
	"context"
	"slices"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	"github.com/yscale-sh/kubagachi/internal/state"
)

var customResourceDefinitionGVR = schema.GroupVersionResource{
	Group:    "apiextensions.k8s.io",
	Version:  "v1",
	Resource: "customresourcedefinitions",
}

func listCustomResourceDefinitions(ctx context.Context, dyn dynamic.Interface) []state.CustomResourceDefinitionView {
	list, err := dyn.Resource(customResourceDefinitionGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	out := make([]state.CustomResourceDefinitionView, 0, len(list.Items))
	for i := range list.Items {
		out = append(out, MapCustomResourceDefinition(&list.Items[i]))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func customResourceDefinitionsEqual(a, b []state.CustomResourceDefinitionView) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x.Name != y.Name || x.Group != y.Group || x.Scope != y.Scope ||
			x.PluralName != y.PluralName || x.SingularName != y.SingularName ||
			x.ListKind != y.ListKind || !slices.Equal(x.Versions, y.Versions) ||
			!slices.Equal(x.ShortNames, y.ShortNames) {
			return false
		}
	}
	return true
}
