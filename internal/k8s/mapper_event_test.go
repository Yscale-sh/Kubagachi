package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestMapEventsNamespace verifies the event feed carries the involved object's
// namespace (falling back to the Event's own namespace) so the UI can filter
// the feed by the selected namespace. Cluster-scoped objects stay namespace-"".
func TestMapEventsNamespace(t *testing.T) {
	events := []*corev1.Event{
		{
			ObjectMeta:     metav1.ObjectMeta{Namespace: "default"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "api-gateway", Namespace: "default"},
			Type:           "Warning",
			Reason:         "BackOff",
			Message:        "Back-off restarting failed container",
		},
		{
			// Involved object has no namespace; fall back to the Event's own.
			ObjectMeta:     metav1.ObjectMeta{Namespace: "kube-system"},
			InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "coredns"},
			Type:           "Normal",
			Reason:         "Pulled",
		},
		{
			// Cluster-scoped object: no namespace anywhere.
			InvolvedObject: corev1.ObjectReference{Kind: "Node", Name: "node-a"},
			Type:           "Warning",
			Reason:         "NodeNotReady",
		},
	}

	got := MapEvents(events)
	if len(got) != len(events) {
		t.Fatalf("MapEvents returned %d events, want %d", len(got), len(events))
	}

	byObject := map[string]string{}
	for _, e := range got {
		byObject[e.Object] = e.Namespace
	}

	cases := map[string]string{
		"Pod/api-gateway": "default",
		"Pod/coredns":     "kube-system",
		"Node/node-a":     "",
	}
	for object, wantNS := range cases {
		if gotNS, ok := byObject[object]; !ok {
			t.Errorf("event for %q missing from output", object)
		} else if gotNS != wantNS {
			t.Errorf("event %q namespace = %q, want %q", object, gotNS, wantNS)
		}
	}
}
