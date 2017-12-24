package instance

type Instance interface {
	Name() string
	Status() State
	ID() string
	Addresses() []string
}
