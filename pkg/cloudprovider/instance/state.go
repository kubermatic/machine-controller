package instance

type State string

const (
	InstanceRunning  State = "running"
	InstanceStarting State = "starting"
	InstancePaused   State = "paused"
	InstanceSaved    State = "saved"
	InstanceStopped  State = "stopped"
	InstanceStopping State = "stopping"
	InstanceError    State = "error"
)
