package k8s

import (
	"context"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

var (
	nodeMetricsGVR = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "nodes"}
	podMetricsGVR  = schema.GroupVersionResource{Group: "metrics.k8s.io", Version: "v1beta1", Resource: "pods"}
)

// usage is a CPU (millicores) + memory (bytes) sample.
type usage struct {
	cpuMilli int64
	memBytes int64
}

// metricsAvailable reports whether the metrics.k8s.io API is served (i.e.
// metrics-server is installed).
func metricsAvailable(cs kubernetes.Interface) bool {
	list, err := cs.Discovery().ServerResourcesForGroupVersion(nodeMetricsGVR.GroupVersion().String())
	if err != nil || list == nil {
		return false
	}
	for _, r := range list.APIResources {
		if r.Name == nodeMetricsGVR.Resource {
			return true
		}
	}
	return false
}

// listNodeMetrics returns per-node usage keyed by node name.
func listNodeMetrics(ctx context.Context, dyn dynamic.Interface) map[string]usage {
	out := map[string]usage{}
	list, err := dyn.Resource(nodeMetricsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for i := range list.Items {
		item := &list.Items[i]
		u, _, _ := unstructured.NestedStringMap(item.Object, "usage")
		out[item.GetName()] = usage{
			cpuMilli: parseMilli(u["cpu"]),
			memBytes: parseBytes(u["memory"]),
		}
	}
	return out
}

// listPodMetrics returns per-pod usage (summed across containers) keyed by
// "namespace/name". ns == "" lists every namespace.
func listPodMetrics(ctx context.Context, dyn dynamic.Interface, ns string) map[string]usage {
	out := map[string]usage{}
	var iface dynamic.ResourceInterface = dyn.Resource(podMetricsGVR)
	if ns != "" {
		iface = dyn.Resource(podMetricsGVR).Namespace(ns)
	}
	list, err := iface.List(ctx, metav1.ListOptions{})
	if err != nil {
		return out
	}
	for i := range list.Items {
		item := &list.Items[i]
		containers, _, _ := unstructured.NestedSlice(item.Object, "containers")
		var sum usage
		for _, c := range containers {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			um, ok := cm["usage"].(map[string]any)
			if !ok {
				continue
			}
			cpu, _ := um["cpu"].(string)
			mem, _ := um["memory"].(string)
			sum.cpuMilli += parseMilli(cpu)
			sum.memBytes += parseBytes(mem)
		}
		out[item.GetNamespace()+"/"+item.GetName()] = sum
	}
	return out
}

func parseMilli(s string) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.MilliValue()
}

func parseBytes(s string) int64 {
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return q.Value()
}
