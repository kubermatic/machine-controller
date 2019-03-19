package manager

// Plugin manages the communication to one plugin. It
// instantiates it and hides all communication to
type Plugin struct {
	filename string
	port     int
}

// newPlugin creates a new plugin manager.
func newPlugin(filename string, port int) (*Plugin, error) {
	return nil, nil
}
