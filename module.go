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
		declares: make(map[string]Declare, 0),
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
		declares map[string]Declare
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
		Driver   string
		External bool
		Codec    string
		Weight   int
		Prefix   string
		Setting  Map
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

// publish 统一真实的发消息
func (this *Module) publish(conn, name string, values ...Map) error {
	if name == "" {
		return ErrInvalidMsg
	}
	if conn == "" {
		conn = this.hashring.Locate(name)
	}

	inst, ok := module.instances[conn]
	if ok == false {
		return ErrInvalidConnection
	}

	var payload Map
	if len(values) > 0 {
		payload = values[0]
	}

	meta := infra.Metadata{Name: name, Payload: payload}

	// 如果有预先，做一下参数处理
	if declare, ok := this.declares[name]; ok {
		if declare.Args != nil {
			value := Map{}
			res := infra.Mapping(declare.Args, meta.Payload, value, declare.Nullable, false)
			if res == nil || res.OK() {
				meta.Payload = value
			}
		}
	}

	var dataBytes []byte

	//根据配置来使用原始编码
	//一般来说连接外部队列，可能会直接发送json出去
	if inst.Config.External {
		bytes, err := infra.Marshal(inst.Config.Codec, &meta.Payload)
		if err != nil {
			return err
		}
		dataBytes = bytes
	} else {
		//内部走meta发消息
		bytes, err := infra.Marshal(inst.Config.Codec, &meta)
		if err != nil {
			return err
		}
		dataBytes = bytes
	}

	realName := inst.Config.Prefix + name
	return inst.connect.Publish(realName, dataBytes)
}
