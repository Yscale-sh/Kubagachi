package state

// EventView is a normalized snapshot of a Kubernetes event for the bottom
// event feed.
type EventView struct {
	Time    string
	Type    string
	Reason  string
	Object  string
	Message string
	// Namespace of the involved object ("" for cluster-scoped objects). Lets
	// the UI filter the event feed by the selected namespace.
	Namespace string
}
