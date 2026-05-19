package event

import (
	"errors"
	"testing"

	. "github.com/infrago/base"
	"github.com/infrago/infra"
)

type recordDriver struct {
	conn *recordConnection
}

func (d *recordDriver) Connect(inst *Instance) (Connection, error) {
	d.conn.inst = inst
	return d.conn, nil
}

type recordConnection struct {
	inst      *Instance
	published int
	subject   string
	data      []byte
}

func (c *recordConnection) Open() error                   { return nil }
func (c *recordConnection) Close() error                  { return nil }
func (c *recordConnection) Start() error                  { return nil }
func (c *recordConnection) Stop() error                   { return nil }
func (c *recordConnection) Register(string, string) error { return nil }
func (c *recordConnection) Publish(name string, data []byte) error {
	c.published++
	c.subject = name
	c.data = data
	return nil
}

func newTestModule() (*Module, *recordConnection) {
	infra.Register("event-test", infra.Codec{
		Encode: func(Any) (Any, error) { return []byte("ok"), nil },
		Decode: func(Any, Any) (Any, error) { return nil, nil },
	})
	conn := &recordConnection{}
	m := &Module{
		configs:   make(map[string]Config),
		drivers:   make(map[string]Driver),
		instances: make(map[string]*Instance),
		events:    make(map[string]Event),
		declares:  make(map[string]Declare),
		filters:   make(map[string]Filter),
		handlers:  make(map[string]Handler),
	}
	m.RegisterDriver("record", &recordDriver{conn: conn})
	m.RegisterConfig("", Config{Driver: "record", Codec: "event-test", Weight: 1})
	return m, conn
}

func TestPublishRejectsInvalidDeclaredPayload(t *testing.T) {
	m, conn := newTestModule()
	m.RegisterEvent("created", Event{
		Args: Vars{
			"id": Var{Required: true},
		},
	})
	m.Setup()
	m.Open()

	err := m.publish("", "created", Map{})
	if err == nil {
		t.Fatal("expected invalid payload error")
	}
	if conn.published != 0 {
		t.Fatalf("expected no publish after invalid payload, got %d", conn.published)
	}
}

func TestPublishUsesConfiguredConnection(t *testing.T) {
	m, conn := newTestModule()
	m.RegisterEvent("created", Event{})
	m.Setup()
	m.Open()

	if err := m.publish("", "created", Map{"id": "1"}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if conn.published != 1 {
		t.Fatalf("expected one publish, got %d", conn.published)
	}
	if conn.subject != "publish.created" {
		t.Fatalf("unexpected subject %q", conn.subject)
	}
}

func TestDefaultConnectionPublishStateErrors(t *testing.T) {
	conn := &defaultConnection{
		events: make(map[string]chan []byte),
		done:   make(chan struct{}),
	}
	if err := conn.Register("publish.created", ""); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if err := conn.Publish("publish.created", []byte("x")); !errors.Is(err, errEventNotRunning) {
		t.Fatalf("expected not running error, got %v", err)
	}

	conn.running = true
	ch := conn.events["publish.created"]
	for i := 0; i < cap(ch); i++ {
		ch <- []byte("x")
	}
	if err := conn.Publish("publish.created", []byte("x")); !errors.Is(err, errEventQueueFull) {
		t.Fatalf("expected queue full error, got %v", err)
	}
}
