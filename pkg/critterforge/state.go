package critterforge

// State names the visual variant of a critter. The set matches the prototype
// sprite sheet: one base ("running") plus six derived states.
type State string

const (
	StateRunning     State = "running"
	StatePending     State = "pending"
	StateCompleted   State = "completed"
	StateCrashLoop   State = "crashloop"
	StateBackOff     State = "backoff"
	StateTerminating State = "terminating"
	StateUnknown     State = "unknown"
)

// AllStates returns every state, running first (since it's the base every
// other state is conditioned on).
func AllStates() []State {
	return []State{
		StateRunning,
		StatePending,
		StateCompleted,
		StateCrashLoop,
		StateBackOff,
		StateTerminating,
		StateUnknown,
	}
}

// DerivedStates returns every state other than running.
func DerivedStates() []State {
	return []State{
		StatePending,
		StateCompleted,
		StateCrashLoop,
		StateBackOff,
		StateTerminating,
		StateUnknown,
	}
}
