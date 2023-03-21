package event

import (
	"sync"

	. "github.com/infrago/base"
	"github.com/infrago/infra"
	"github.com/infrago/util"

	"github.com/panjf2000/ants/v2"
)

func init() {
	infra.Mount(module)
}

var (
	module = &Module{
		configs:   make(map[string]Config, 0),
		drivers:   make(map[string]Driver, 0),
		instances: make(map[string]*Instance, 0),

		events:   make(map[string]Event, 0),
		notices:  make(map[string]Notice, 0),
		filters:  make(map[string]Filter, 0),
		handlers: make(map[string]Handler, 0),
	}
)

type (
	Module struct {
		mutex sync.Mutex
		pool  *ants.Pool

		connected, initialized, launched bool

		configs map[string]Config
		drivers map[string]Driver

		events   map[string]Event
		notices  map[string]Notice
		filters  map[string]Filter
		handlers map[string]Handler

		relates map[string]string

		serveFilters    []ctxFunc
		requestFilters  []ctxFunc
		executeFilters  []ctxFunc
		responseFilters []ctxFunc

		foundHandlers  []ctxFunc
		errorHandlers  []ctxFunc
		failedHandlers []ctxFunc
		deniedHandlers []ctxFunc

		instances map[string]*Instance

		weights  map[string]int
		hashring *util.HashRing
	}

	Configs map[string]Config
	Config  struct {
		Driver  string
		Codec   string
		Weight  int
		Prefix  string
		Setting Map
	}
	Instance struct {
		module  *Module
		Name    string
		Config  Config
		connect Connect
	}
)

// Driver 注册驱动
func (module *Module) Driver(name string, driver Driver) {
	module.mutex.Lock()
	defer module.mutex.Unlock()

	if driver == nil {
		panic("Invalid event driver: " + name)
	}

	if infra.Override() {
		module.drivers[name] = driver
	} else {
		if module.drivers[name] == nil {
			module.drivers[name] = driver
		}
	}
}

func (this *Module) Config(name string, config Config) {
	this.mutex.Lock()
	defer this.mutex.Unlock()

	if name == "" {
		name = infra.DEFAULT
	}

	if infra.Override() {
		this.configs[name] = config
	} else {
		if _, ok := this.configs[name]; ok == false {
			this.configs[name] = config
		}
	}
}
func (this *Module) Configs(config Configs) {
	for key, val := range config {
		this.Config(key, val)
	}
}

// notify 统一真实的发消息
func (this *Module) notify(conn, name string, values ...Map) error {
	if name == "" {
		return errInvalidMsg
	}
	if conn == "" {
		conn = this.hashring.Locate(name)
	}

	inst, ok := module.instances[conn]
	if ok == false {
		return errInvalidConnection
	}

	var payload Map
	if len(values) > 0 {
		payload = values[0]
	}

	meta := infra.Metadata{Name: name, Payload: payload}

	// 如果有预先定义消息，做一下参数处理
	if notice, ok := this.notices[name]; ok {
		if notice.Args != nil {
			value := Map{}
			res := infra.Mapping(notice.Args, meta.Payload, value, notice.Nullable, false)
			if res == nil || res.OK() {
				meta.Payload = value
			}
		}
	}

	bytes, err := infra.Marshal(inst.Config.Codec, &meta)
	if err != nil {
		return err
	}

	realName := inst.Config.Prefix + name
	return inst.connect.Notify(realName, bytes)
}
