package alternatetwo

// Task represents a unit of work submitted to the event loop.
type Task struct {
	Fn func()
}
