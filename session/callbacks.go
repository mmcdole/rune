package session

import "fmt"

// CallbackManager manages one-shot picker callbacks.
type CallbackManager struct {
	registry map[string]func(string)
	nextID   int
}

// NewCallbackManager creates a new callback manager.
func NewCallbackManager() *CallbackManager {
	return &CallbackManager{
		registry: make(map[string]func(string)),
	}
}

// Register stores a callback and returns its ID.
func (c *CallbackManager) Register(fn func(string)) string {
	c.nextID++
	id := fmt.Sprintf("p%d", c.nextID)
	c.registry[id] = fn
	return id
}

// Execute runs and removes a callback by ID.
// Returns true if the callback was found and executed.
func (c *CallbackManager) Execute(id string, value string, accepted bool) bool {
	cb, ok := c.registry[id]
	if !ok {
		return false
	}
	delete(c.registry, id) // One-shot
	if accepted && cb != nil {
		cb(value)
	}
	return true
}
