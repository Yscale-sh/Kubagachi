package state

// EventView is a normalized snapshot of a Kubernetes event for the bottom
// event feed.
type EventView struct {
	Time    string
	Type    string
	Reason  string
	Object  string
	Message string
}
