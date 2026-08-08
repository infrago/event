package event

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

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
	var payloadErr *PayloadError
	if !errors.As(err, &payloadErr) {
		t.Fatalf("expected payload error type, got %T", err)
	}
	if !errors.Is(err, ErrInvalidEventPayload) {
		t.Fatalf("expected invalid payload sentinel, got %v", err)
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

func TestPublishAllowsDeclaredProducerWithoutConsumer(t *testing.T) {
	m, conn := newTestModule()
	m.RegisterDeclare("created", Declare{
		Args: Vars{
			"id": Var{Required: true},
		},
	})
	m.Setup()
	m.Open()

	if err := m.publish("", "created", Map{"id": "1"}); err != nil {
		t.Fatalf("publish from declared producer failed: %v", err)
	}
	if conn.published != 1 {
		t.Fatalf("expected one publish, got %d", conn.published)
	}
	if conn.subject != "publish.created" {
		t.Fatalf("unexpected subject %q", conn.subject)
	}
}

func TestPublishRejectsInvalidEventName(t *testing.T) {
	m, _ := newTestModule()
	m.RegisterEvent("created", Event{})
	m.Setup()
	m.Open()

	if err := m.publish("", " publish.created ", Map{}); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected invalid event error, got %v", err)
	}
}

func TestSetupNormalizesPrefixAndQueuePolicy(t *testing.T) {
	m, conn := newTestModule()
	m.configs[infra.DEFAULT] = Config{Driver: "record", Codec: "event-test", Prefix: "app", QueuePolicy: "DROP", Weight: 1}
	m.RegisterEvent("created", Event{})
	m.Setup()
	m.Open()

	if err := m.publish("", "created", Map{}); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	if conn.subject != "app.publish.created" {
		t.Fatalf("unexpected subject %q", conn.subject)
	}
	if got := m.configs[infra.DEFAULT].QueuePolicy; got != "drop" {
		t.Fatalf("unexpected queue policy %q", got)
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

func TestInstanceWorkerPoolExecutesSubmittedTasks(t *testing.T) {
	inst := &Instance{Config: Config{Workers: 2, Queue: 2}}
	inst.startWorkers()
	defer inst.stopWorkers()

	done := make(chan struct{})
	var count atomic.Int32
	for i := 0; i < 2; i++ {
		inst.Submit(func() {
			if count.Add(1) == 2 {
				close(done)
			}
		})
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker pool did not execute submitted tasks")
	}
}

func TestInstanceSubmitSyncPolicyRunsInline(t *testing.T) {
	inst := &Instance{Config: Config{QueuePolicy: "sync"}}
	ran := false
	inst.Submit(func() {
		ran = true
	})
	if !ran {
		t.Fatal("sync queue policy did not run inline")
	}
}

func TestContextRetryMarksResultRetriable(t *testing.T) {
	ctx := &Context{
		inst: &Instance{},
		Meta: infra.NewMeta(),
	}
	ctx.Retry(infra.Fail)
	if res := ctx.Meta.Result(); !infra.IsRetry(res) {
		t.Fatalf("expected retry result, got %v", res)
	}
}
