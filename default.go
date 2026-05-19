package event

import (
	"errors"
	"sync"

	"github.com/infrago/infra"
)

func init() {
	infra.Register(infra.DEFAULT, &defaultDriver{})
}

var (
	errEventRunning    = errors.New("event is running")
	errEventNotRunning = errors.New("event is not running")
	errEventQueueFull  = errors.New("event queue is full")
)

type (
	defaultDriver struct{}

	defaultConnection struct {
		mutex    sync.RWMutex
		running  bool
		instance *Instance
		events   map[string]chan []byte
		done     chan struct{}
		wg       sync.WaitGroup
	}
)

func (d *defaultDriver) Connect(inst *Instance) (Connection, error) {
	return &defaultConnection{
		instance: inst,
		events:   make(map[string]chan []byte, 0),
		done:     make(chan struct{}),
	}, nil
}

func (c *defaultConnection) Open() error { return nil }
func (c *defaultConnection) Close() error {
	_ = c.Stop()
	return nil
}

func (c *defaultConnection) Register(name, _ string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if _, ok := c.events[name]; !ok {
		c.events[name] = make(chan []byte, 64)
	}
	return nil
}

func (c *defaultConnection) Start() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.running {
		return errEventRunning
	}
	for name, ch := range c.events {
		eventName := name
		eventCh := ch
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			for {
				select {
				case data := <-eventCh:
					c.instance.Serve(eventName, data)
				case <-c.done:
					return
				}
			}
		}()
	}
	c.running = true
	return nil
}

func (c *defaultConnection) Stop() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if !c.running {
		return errEventNotRunning
	}
	close(c.done)
	c.wg.Wait()
	c.done = make(chan struct{})
	c.running = false
	return nil
}

func (c *defaultConnection) Publish(name string, data []byte) error {
	c.mutex.RLock()
	ch := c.events[name]
	running := c.running
	done := c.done
	c.mutex.RUnlock()
	if ch == nil {
		return errInvalidEvent
	}
	if !running {
		return errEventNotRunning
	}
	select {
	case ch <- data:
		return nil
	case <-done:
		return errEventNotRunning
	default:
		return errEventQueueFull
	}
}
