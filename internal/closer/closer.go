package closer

import (
	"errors"
	"fmt"
	"sync"
)

var (
	ErrClosed  = errors.New("closer is already closed")
	ErrNilFunc = errors.New("close function is nil")
)

type closeFunc struct {
	name string
	fn   func() error
}

type Closer struct {
	mu        sync.Mutex
	functions []closeFunc
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

func New() *Closer {
	return &Closer{}
}

func (c *Closer) Add(name string, fn func() error) error {
	if fn == nil {
		return ErrNilFunc
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return ErrClosed
	}

	c.functions = append(c.functions, closeFunc{name: name, fn: fn})
	return nil
}

func (c *Closer) Close() error {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		functions := append([]closeFunc(nil), c.functions...)
		c.mu.Unlock()

		var closeErrors []error
		for i := len(functions) - 1; i >= 0; i-- {
			if err := functions[i].fn(); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close %s: %w", functions[i].name, err))
			}
		}
		c.closeErr = errors.Join(closeErrors...)
	})

	return c.closeErr
}
