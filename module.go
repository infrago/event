package event

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	. "github.com/infrago/base"
	"github.com/infrago/infra"
	"github.com/infrago/util"
)

var (
	ErrInvalidEvent        = errors.New("invalid event name")
	ErrNoConnection        = errors.New("invalid event connection")
	ErrInvalidEventPayload = errors.New("invalid event payload")

	errInvalidEvent        = ErrInvalidEvent
	errNoConnection        = ErrNoConnection
	errInvalidEventPayload = ErrInvalidEventPayload
)

const (
	broadcastSubjectPrefix = "broadcast."
	publishSubjectPrefix   = "publish."
)

func init() {
	infra.Mount(module)
}

var module = &Module{
	configs:   make(map[string]Config, 0),
	drivers:   make(map[string]Driver, 0),
	instances: make(map[string]*Instance, 0),

	events:   make(map[string]Event, 0),
	declares: make(map[string]Declare, 0),
	filters:  make(map[string]Filter, 0),
	handlers: make(map[string]Handler, 0),
}

type (
	Module struct {
		mutex sync.RWMutex

		opened  bool
		started bool

		configs map[string]Config
		drivers map[string]Driver

		events   map[string]Event
		declares map[string]Declare
		filters  map[string]Filter
		handlers map[string]Handler

		serveFilters    []ctxFunc
		requestFilters  []ctxFunc
		executeFilters  []ctxFunc
		responseFilters []ctxFunc
		foundHandlers   []ctxFunc
		errorHandlers   []ctxFunc
		failedHandlers  []ctxFunc
		deniedHandlers  []ctxFunc

		instances map[string]*Instance
		weights   map[string]int
		hashring  *util.HashRing
	}

	Configs map[string]Config

	Config struct {
		Driver       string
		External     bool
		Codec        string
		Weight       int
		Workers      int
		Queue        int
		QueuePolicy  string
		DrainTimeout time.Duration
		Prefix       string
		Setting      Map
	}

	PayloadError struct {
		Event  string
		Result Res
	}

	msgEnvelope struct {
		Name     string         `json:"name"`
		Metadata infra.Metadata `json:"metadata"`
		Payload  Map            `json:"payload"`
	}
)

func (err *PayloadError) Error() string {
	if err == nil {
		return ""
	}
	if err.Result == nil {
		return errInvalidEventPayload.Error()
	}
	if err.Event == "" {
		return err.Result.Error()
	}
	return fmt.Sprintf("%s: %s", err.Event, err.Result.Error())
}

func (err *PayloadError) Unwrap() error {
	return errInvalidEventPayload
}

func (m *Module) Register(name string, value Any) {
	switch v := value.(type) {
	case Driver:
		m.RegisterDriver(name, v)
	case Config:
		m.RegisterConfig(name, v)
	case Configs:
		m.RegisterConfigs(v)
	case Event:
		m.RegisterEvent(name, v)
	case Events:
		m.RegisterEvents(name, v)
	case Declare:
		m.RegisterDeclare(name, v)
	case Filter:
		m.RegisterFilter(name, v)
	case Handler:
		m.RegisterHandler(name, v)
	}
}

func (m *Module) RegisterEvents(prefix string, events Events) {
	for name, event := range events {
		target := name
		if prefix != "" {
			target = prefix + "." + name
		}
		m.RegisterEvent(target, event)
	}
}

func (m *Module) RegisterDriver(name string, driver Driver) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if name == "" {
		name = infra.DEFAULT
	}
	if driver == nil {
		panic("invalid event driver: " + name)
	}
	if _, ok := m.drivers[name]; ok {
		panic("event driver already registered: " + name)
	}
	m.drivers[name] = driver
}

func (m *Module) RegisterConfig(name string, cfg Config) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.opened || m.started {
		return
	}
	if name == "" {
		name = infra.DEFAULT
	}
	if _, ok := m.configs[name]; ok {
		panic("event config already registered: " + name)
	}
	m.configs[name] = cfg
}

func (m *Module) RegisterConfigs(configs Configs) {
	for name, cfg := range configs {
		m.RegisterConfig(name, cfg)
	}
}

func (m *Module) Config(global Map) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.opened || m.started {
		return
	}

	cfgAny, ok := global["event"]
	if !ok {
		return
	}
	cfgMap, ok := cfgAny.(Map)
	if !ok || cfgMap == nil {
		return
	}

	root := Map{}
	for key, val := range cfgMap {
		if conf, ok := val.(Map); ok && key != "setting" {
			m.configure(key, conf)
		} else {
			root[key] = val
		}
	}
	if len(root) > 0 {
		m.configure(infra.DEFAULT, root)
	}
}

func (m *Module) configure(name string, conf Map) {
	cfg := Config{
		Driver: infra.DEFAULT,
		Codec:  infra.GOB,
		Weight: 1,
	}
	if existed, ok := m.configs[name]; ok {
		cfg = existed
	}

	if v, ok := conf["driver"].(string); ok && v != "" {
		cfg.Driver = v
	}
	if v, ok := conf["external"].(bool); ok {
		cfg.External = v
	}
	if v, ok := conf["codec"].(string); ok && v != "" {
		cfg.Codec = v
	}
	if v, ok := conf["prefix"].(string); ok {
		cfg.Prefix = normalizePrefix(v)
	}
	if v, ok := conf["weight"].(int); ok {
		cfg.Weight = v
	}
	if v, ok := conf["weight"].(int64); ok {
		cfg.Weight = int(v)
	}
	if v, ok := conf["weight"].(float64); ok {
		cfg.Weight = int(v)
	}
	if v, ok := conf["weight"].(string); ok {
		if w, err := strconv.Atoi(v); err == nil {
			cfg.Weight = w
		}
	}
	cfg.Workers = IntSetting(conf, "workers", cfg.Workers)
	cfg.Queue = IntSetting(conf, "queue", cfg.Queue)
	cfg.QueuePolicy = StringSetting(conf, "queue_policy", cfg.QueuePolicy)
	cfg.DrainTimeout = DurationSetting(conf, "drain_timeout", cfg.DrainTimeout)
	if v, ok := conf["setting"].(Map); ok {
		cfg.Setting = v
	}

	m.configs[name] = cfg
}

func (m *Module) Setup() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if len(m.configs) == 0 {
		m.configs[infra.DEFAULT] = Config{
			Driver: infra.DEFAULT,
			Codec:  infra.GOB,
			Weight: 1,
		}
	}

	for name, cfg := range m.configs {
		if cfg.Driver == "" {
			cfg.Driver = infra.DEFAULT
		}
		if cfg.Codec == "" {
			cfg.Codec = infra.GOB
		}
		if cfg.Weight == 0 {
			cfg.Weight = 1
		}
		if cfg.Workers < 0 {
			cfg.Workers = 0
		}
		if cfg.Queue < 0 {
			cfg.Queue = 0
		}
		cfg.Prefix = normalizePrefix(cfg.Prefix)
		cfg.QueuePolicy = normalizeQueuePolicy(cfg.QueuePolicy)
		m.configs[name] = cfg
	}

	m.serveFilters = make([]ctxFunc, 0)
	m.requestFilters = make([]ctxFunc, 0)
	m.executeFilters = make([]ctxFunc, 0)
	m.responseFilters = make([]ctxFunc, 0)
	for _, f := range m.filters {
		if f.Serve != nil {
			m.serveFilters = append(m.serveFilters, f.Serve)
		}
		if f.Request != nil {
			m.requestFilters = append(m.requestFilters, f.Request)
		}
		if f.Execute != nil {
			m.executeFilters = append(m.executeFilters, f.Execute)
		}
		if f.Response != nil {
			m.responseFilters = append(m.responseFilters, f.Response)
		}
	}

	m.foundHandlers = make([]ctxFunc, 0)
	m.errorHandlers = make([]ctxFunc, 0)
	m.failedHandlers = make([]ctxFunc, 0)
	m.deniedHandlers = make([]ctxFunc, 0)
	for _, h := range m.handlers {
		if h.Found != nil {
			m.foundHandlers = append(m.foundHandlers, h.Found)
		}
		if h.Error != nil {
			m.errorHandlers = append(m.errorHandlers, h.Error)
		}
		if h.Failed != nil {
			m.failedHandlers = append(m.failedHandlers, h.Failed)
		}
		if h.Denied != nil {
			m.deniedHandlers = append(m.deniedHandlers, h.Denied)
		}
	}
}

func (m *Module) Open() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.opened {
		return
	}

	weights := make(map[string]int, 0)
	role := infra.Identity().Role
	if role == "" {
		role = infra.Identity().Profile
	}
	if role == "" {
		role = infra.INFRAGO
	}
	for name, cfg := range m.configs {
		driver, ok := m.drivers[cfg.Driver]
		if !ok || driver == nil {
			panic("missing event driver: " + cfg.Driver)
		}

		inst := &Instance{
			Name:    name,
			Config:  cfg,
			Setting: cfg.Setting,
		}

		conn, err := driver.Connect(inst)
		if err != nil {
			panic("failed to connect event: " + err.Error())
		}
		if err := conn.Open(); err != nil {
			panic("failed to open event: " + err.Error())
		}

		for evName, ev := range m.events {
			if ev.Connect == "" || ev.Connect == "*" || ev.Connect == name {
				if err := conn.Register(cfg.Prefix+broadcastSubjectPrefix+evName, ""); err != nil {
					panic("failed to register event broadcast: " + err.Error())
				}
				if err := conn.Register(cfg.Prefix+publishSubjectPrefix+evName, role); err != nil {
					panic("failed to register event: " + err.Error())
				}
			}
		}

		inst.conn = conn
		m.instances[name] = inst
		if cfg.Weight > 0 {
			weights[name] = cfg.Weight
		}
	}

	m.weights = weights
	m.hashring = util.NewHashRing(weights)
	m.opened = true
}

func (m *Module) Start() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if m.started {
		return
	}

	for _, inst := range m.instances {
		inst.startWorkers()
		if err := inst.conn.Start(); err != nil {
			inst.stopWorkers()
			panic("failed to start event: " + err.Error())
		}
	}

	infra.Log(infra.LogLevelInfo, "event", "module started", Map{
		"connections": len(m.instances), "events": len(m.events),
	})
	m.started = true
}

func (m *Module) Stop() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.started {
		return
	}
	for _, inst := range m.instances {
		_ = inst.conn.Stop()
		inst.stopWorkers()
	}
	m.started = false
}

func (m *Module) Close() {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if !m.opened {
		return
	}
	for _, inst := range m.instances {
		if inst.conn != nil {
			_ = inst.conn.Close()
			inst.stopWorkers()
			inst.conn = nil
		}
	}
	m.instances = make(map[string]*Instance, 0)
	m.weights = nil
	m.hashring = nil
	m.opened = false
}

func (m *Module) publish(connName, name string, values ...Map) error {
	return m.publishMode(connName, publishSubjectPrefix, name, values...)
}

func (m *Module) broadcast(connName, name string, values ...Map) error {
	return m.publishMode(connName, broadcastSubjectPrefix, name, values...)
}

func (m *Module) publishMode(connName, mode, name string, values ...Map) error {
	return m.publishModeMeta(nil, connName, mode, name, values...)
}

func (m *Module) publishMeta(meta *infra.Meta, connName, name string, values ...Map) error {
	return m.publishModeMeta(meta, connName, publishSubjectPrefix, name, values...)
}

func (m *Module) broadcastMeta(meta *infra.Meta, connName, name string, values ...Map) error {
	return m.publishModeMeta(meta, connName, broadcastSubjectPrefix, name, values...)
}

func (m *Module) publishModeMeta(meta *infra.Meta, connName, mode, name string, values ...Map) error {
	if err := validateEventName(name); err != nil {
		return err
	}

	m.mutex.RLock()
	eventCfg, registered := m.events[name]
	_, declared := m.declares[name]
	if !registered && !declared {
		m.mutex.RUnlock()
		return errInvalidEvent
	}

	if connName == "" {
		if eventCfg.Connect != "" && eventCfg.Connect != "*" {
			connName = eventCfg.Connect
		} else {
			if m.hashring == nil {
				m.mutex.RUnlock()
				return errNoConnection
			}
			connName = m.hashring.Locate(name)
		}
	}
	inst, ok := m.instances[connName]
	if !ok || inst == nil || inst.conn == nil {
		m.mutex.RUnlock()
		return errNoConnection
	}
	if eventCfg.Connect != "" && eventCfg.Connect != "*" && eventCfg.Connect != connName {
		m.mutex.RUnlock()
		return errNoConnection
	}
	dec := m.declares[name]
	conn := inst.conn
	cfg := inst.Config
	instName := inst.Name
	m.mutex.RUnlock()

	var payload Map
	if len(values) > 0 {
		payload = values[0]
	}
	if payload == nil {
		payload = Map{}
	}

	if dec.Args != nil {
		mapped := Map{}
		res := infra.Mapping(dec.Args, payload, mapped, dec.Nullable, false)
		if res != nil && res.Fail() {
			return &PayloadError{Event: name, Result: res}
		}
		payload = mapped
	}

	var data []byte
	if cfg.External {
		bytes, err := infra.Marshal(cfg.Codec, payload)
		if err != nil {
			return err
		}
		data = bytes
	} else {
		metadata := infra.NewMeta().Metadata()
		if meta != nil {
			metadata = meta.Metadata()
		}
		body := msgEnvelope{
			Name:     name,
			Metadata: metadata,
			Payload:  payload,
		}
		bytes, err := infra.Marshal(cfg.Codec, body)
		if err != nil {
			return err
		}
		data = bytes
	}

	prefix := cfg.Prefix
	subject := prefix + mode + name

	traceMeta := meta
	if traceMeta == nil {
		traceMeta = infra.NewMeta()
	}
	span := traceMeta.Begin("event:"+name, infra.TraceAttrs("infrago", infra.TraceKindEvent, name, Map{
		"module":     "event",
		"connection": instName,
		"operation":  "publish",
		"mode":       strings.TrimSuffix(mode, "."),
		"codec":      cfg.Codec,
		"external":   cfg.External,
		"bytes":      len(data),
	}))
	err := conn.Publish(subject, data)
	if err != nil {
		span.End(err)
		return err
	}
	span.End()
	return nil
}

func (m *Module) eventConnectable(connName, name string) bool {
	ev, ok := m.events[name]
	if !ok {
		return false
	}
	return ev.Connect == "" || ev.Connect == "*" || ev.Connect == connName
}

func (inst *Instance) Submit(next func()) {
	if next == nil {
		return
	}
	policy := normalizeQueuePolicy(inst.Config.QueuePolicy)
	if inst.tasks != nil && inst.workerStop != nil {
		switch policy {
		case "drop":
			select {
			case inst.tasks <- next:
			case <-inst.workerStop:
			default:
			}
		case "sync":
			next()
		case "async":
			select {
			case inst.tasks <- next:
			case <-inst.workerStop:
			default:
				go next()
			}
		default:
			select {
			case inst.tasks <- next:
			case <-inst.workerStop:
			}
		}
		return
	}
	if policy == "sync" {
		next()
		return
	}
	go next()
}

func (inst *Instance) startWorkers() {
	if inst.Config.Workers <= 0 || inst.tasks != nil {
		return
	}
	queueSize := inst.Config.Queue
	if queueSize <= 0 {
		queueSize = inst.Config.Workers * 64
	}
	inst.tasks = make(chan func(), queueSize)
	inst.workerStop = make(chan struct{})
	for i := 0; i < inst.Config.Workers; i++ {
		inst.workerWg.Add(1)
		go func() {
			defer inst.workerWg.Done()
			for {
				select {
				case next := <-inst.tasks:
					if next != nil {
						next()
					}
				case <-inst.workerStop:
					return
				}
			}
		}()
	}
}

func (inst *Instance) stopWorkers() {
	if inst.workerStop == nil {
		return
	}
	if inst.Config.DrainTimeout > 0 && inst.tasks != nil {
		timer := time.NewTimer(inst.Config.DrainTimeout)
		ticker := time.NewTicker(10 * time.Millisecond)
		drained := false
		for !drained {
			if len(inst.tasks) == 0 {
				drained = true
				break
			}
			select {
			case <-timer.C:
				drained = true
			case <-ticker.C:
			}
		}
		ticker.Stop()
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	close(inst.workerStop)
	inst.workerWg.Wait()
	inst.tasks = nil
	inst.workerStop = nil
}

func (inst *Instance) Serve(name string, data []byte) {
	go inst.serving(name, data)
}

func (inst *Instance) ServeSync(name string, data []byte) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	res := inst.serving(name, data)
	return res == nil || !res.Fail()
}

func (inst *Instance) serving(name string, data []byte) (ret Res) {
	start := time.Now()
	if inst.Config.Prefix != "" && len(name) >= len(inst.Config.Prefix) && name[:len(inst.Config.Prefix)] == inst.Config.Prefix {
		name = name[len(inst.Config.Prefix):]
	}
	if len(name) >= len(broadcastSubjectPrefix) && name[:len(broadcastSubjectPrefix)] == broadcastSubjectPrefix {
		name = name[len(broadcastSubjectPrefix):]
	} else if len(name) >= len(publishSubjectPrefix) && name[:len(publishSubjectPrefix)] == publishSubjectPrefix {
		name = name[len(publishSubjectPrefix):]
	}

	ctx := &Context{
		inst:    inst,
		Meta:    infra.NewMeta(),
		nexts:   make([]ctxFunc, 0),
		Setting: Map{},
		Value:   Map{},
		Args:    Map{},
		Locals:  Map{},
	}
	ctx.Name = name
	inst.loadEvent(ctx, name)

	var decodeErr error
	if inst.Config.External {
		payload := Map{}
		if err := infra.Unmarshal(inst.Config.Codec, data, &payload); err == nil {
			ctx.Value = payload
		} else {
			decodeErr = errors.Join(errInvalidEventPayload, err)
		}
	} else {
		env := msgEnvelope{}
		if err := infra.Unmarshal(inst.Config.Codec, data, &env); err == nil {
			ctx.Metadata(env.Metadata)
			if env.Payload != nil {
				ctx.Value = env.Payload
			}
			if env.Name != "" {
				ctx.Name = env.Name
				inst.loadEvent(ctx, env.Name)
			}
		} else {
			decodeErr = errors.Join(errInvalidEventPayload, err)
		}
	}

	span := ctx.Begin("event:"+ctx.Name, infra.TraceAttrs("infrago", infra.TraceKindEvent, ctx.Name, Map{
		"module":     "event",
		"connection": inst.Name,
		"operation":  "consume",
		"codec":      inst.Config.Codec,
		"external":   inst.Config.External,
		"bytes":      len(data),
	}))
	defer func() {
		if r := recover(); r != nil {
			ret = infra.ErrorResult(fmt.Errorf("event panic: %v", r))
		}
		status := infra.OK.Status()
		retry := false
		if ret != nil && ret.Fail() {
			status = ret.Status()
			retry = infra.IsRetry(ret)
		}
		_ = ctx.Trace("event:"+ctx.Name, infra.TraceAttrs("infrago", infra.TraceKindEvent, ctx.Name, Map{
			"module":      "event",
			"connection":  inst.Name,
			"operation":   "consume.done",
			"status":      status,
			"retry":       retry,
			"duration_ms": time.Since(start).Milliseconds(),
		}))
		if ret != nil && ret.Fail() {
			span.End(ret)
		} else {
			span.End()
		}
	}()

	if decodeErr != nil {
		ctx.Error(infra.ErrorResult(decodeErr))
	} else {
		inst.open(ctx)
	}
	ret = ctx.Result()
	return ret
}

func validateEventName(name string) error {
	if name == "" || strings.TrimSpace(name) != name {
		return errInvalidEvent
	}
	if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") || strings.Contains(name, "..") {
		return errInvalidEvent
	}
	if strings.HasPrefix(name, publishSubjectPrefix) || strings.HasPrefix(name, broadcastSubjectPrefix) {
		return errInvalidEvent
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return errInvalidEvent
		}
	}
	return nil
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	if strings.HasSuffix(prefix, ".") || strings.HasSuffix(prefix, ":") || strings.HasSuffix(prefix, "/") {
		return prefix
	}
	return prefix + "."
}

func normalizeQueuePolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "drop", "sync", "async":
		return strings.ToLower(strings.TrimSpace(policy))
	case "block":
		return "block"
	default:
		return ""
	}
}

func (inst *Instance) loadEvent(ctx *Context, name string) {
	if cfg, ok := module.events[name]; ok {
		ctx.Config = &cfg
		ctx.Setting = cfg.Setting
		return
	}
	ctx.Config = nil
	ctx.Setting = Map{}
}

func (inst *Instance) open(ctx *Context) {
	ctx.clear()
	ctx.next(module.serveFilters...)
	ctx.next(inst.serve)
	ctx.Next()
}

func (inst *Instance) serve(ctx *Context) {
	ctx.clear()
	ctx.next(module.requestFilters...)
	ctx.next(inst.request)
	ctx.Next()
	inst.response(ctx)
}

func (inst *Instance) request(ctx *Context) {
	ctx.clear()
	ctx.next(inst.finding)
	ctx.next(inst.authorizing)
	ctx.next(inst.arguing)
	ctx.next(inst.execute)
	ctx.Next()
}

func (inst *Instance) execute(ctx *Context) {
	ctx.clear()
	ctx.next(module.executeFilters...)
	if ctx.Config != nil {
		if len(ctx.Config.Actions) > 0 {
			ctx.next(ctx.Config.Actions...)
		}
		if ctx.Config.Action != nil {
			ctx.next(ctx.Config.Action)
		}
	}
	ctx.Next()
}

func (inst *Instance) response(ctx *Context) {
	ctx.clear()
	ctx.next(module.responseFilters...)
	ctx.Next()
	inst.body(ctx)
}

func (inst *Instance) finding(ctx *Context) {
	if ctx.Config == nil {
		ctx.Found()
		return
	}
	ctx.Next()
}

func (inst *Instance) authorizing(ctx *Context) {
	ctx.Next()
}

func (inst *Instance) arguing(ctx *Context) {
	if ctx.Config != nil && ctx.Config.Args != nil {
		argsValue := Map{}
		res := infra.Mapping(ctx.Config.Args, ctx.Value, argsValue, ctx.Config.Nullable, false, ctx.Timezone())
		if res != nil && res.Fail() {
			ctx.Failed(res)
			return
		}
		for k, v := range argsValue {
			ctx.Args[k] = v
		}
	}
	ctx.Next()
}

func (inst *Instance) found(ctx *Context) {
	ctx.clear()
	if ctx.Config != nil && ctx.Config.Found != nil {
		ctx.next(ctx.Config.Found)
	}
	ctx.next(module.foundHandlers...)
	ctx.Next()
}

func (inst *Instance) error(ctx *Context) {
	ctx.clear()
	if ctx.Config != nil && ctx.Config.Error != nil {
		ctx.next(ctx.Config.Error)
	}
	ctx.next(module.errorHandlers...)
	ctx.Next()
}

func (inst *Instance) failed(ctx *Context) {
	ctx.clear()
	if ctx.Config != nil && ctx.Config.Failed != nil {
		ctx.next(ctx.Config.Failed)
	}
	ctx.next(module.failedHandlers...)
	ctx.Next()
}

func (inst *Instance) denied(ctx *Context) {
	ctx.clear()
	if ctx.Config != nil && ctx.Config.Denied != nil {
		ctx.next(ctx.Config.Denied)
	}
	ctx.next(module.deniedHandlers...)
	ctx.Next()
}

func (inst *Instance) body(_ *Context) {}
