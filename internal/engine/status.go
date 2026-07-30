package engine

// Status reports what an engine knows about a component right now.
type Status struct {
	// State is the component's lifecycle phase.
	State State

	// ExitCode is set when State is Stopped or Failed. It is how the
	// caller distinguishes a clean exit (0) from a crash (non-zero) — the
	// difference between "the job finished" and "the app fell over".
	ExitCode int

	// Message is a human-readable detail, especially for Failed: an OOM
	// kill, an image that could not be pulled, a start command that did
	// not exist. This is what turns a mystery into a fixable error.
	Message string
}

// State is the lifecycle phase of a component. These are engine-neutral
// on purpose — every backend, whether it runs containers or microVMs or
// processes, can map itself onto these five.
type State int

const (
	// StateUnknown means the engine could not determine the state. Always
	// treat this as a problem to investigate, never as "probably fine".
	StateUnknown State = iota

	// StateStarting StateStarting: created and launched, but not yet passing health
	// checks. Traffic must not be routed to a component in this state.
	StateStarting

	// StateRunning StateRunning: up and healthy. This is the only state in which a
	// component should receive traffic.
	StateRunning

	// StateStopped StateStopped: exited cleanly, exit code 0. Expected for a finished
	// job; unexpected for a long-running service.
	StateStopped

	// StateFailed StateFailed: exited non-zero, crashed, or could not start. Message
	// should explain why.
	StateFailed
)

// String makes State readable in logs and the debug TUI.
func (s State) String() string {
	switch s {
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Healthy reports whether a component is up and able to serve.
// A convenience so callers write status.Healthy() rather than
// comparing against StateRunning everywhere.
func (s Status) Healthy() bool {
	return s.State == StateRunning
}
