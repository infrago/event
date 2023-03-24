package event

import (
	"time"

	. "github.com/infrago/base"
	"github.com/infrago/infra"
	"github.com/infrago/util"

	"github.com/panjf2000/ants/v2"
)

func (this *Module) Register(name string, value Any) {
	switch val := value.(type) {
	case Driver:
		this.Driver(name, val)
	case Config:
		this.Config(name, val)
	case Configs:
		this.Configs(val)
	case Event:
		this.Event(name, val)
	case Declare:
		this.Declare(name, val)
	case Filter:
		this.Filter(name, val)
	case Handler:
		this.Handler(name, val)
	}
}

func (this *Module) configure(name string, config Map) {
	cfg := Config{
		Driver: infra.DEFAULT, Weight: 1, Codec: infra.GOB,
	}
	//如果已经存在了，用现成的改写
	if vv, ok := this.configs[name]; ok {
		cfg = vv
	}

	if driver, ok := config["driver"].(string); ok {
		cfg.Driver = driver
	}

	if external, ok := config["external"].(bool); ok {
		cfg.External = external
	}

	//分配权重
	if weight, ok := config["weight"].(int); ok {
		cfg.Weight = weight
	}
	if weight, ok := config["weight"].(int64); ok {
		cfg.Weight = int(weight)
	}
	if weight, ok := config["weight"].(float64); ok {
		cfg.Weight = int(weight)
	}

	if weight, ok := config["weight"].(float64); ok {
		cfg.Weight = int(weight)
	}

	if setting, ok := config["setting"].(Map); ok {
		cfg.Setting = setting
	}

	//保存配置
	this.configs[name] = cfg
}
func (this *Module) Configure(global Map) {
	var config Map
	if vvv, ok := global["event"].(Map); ok {
		config = vvv
	}
	if config == nil {
		return
	}

	//记录上一层的配置，如果有的话
	defaultConfig := Map{}

	for key, val := range config {
		if conf, ok := val.(Map); ok {
			this.configure(key, conf)
		} else {
			defaultConfig[key] = val
		}
	}

	if len(defaultConfig) > 0 {
		this.configure(infra.DEFAULT, defaultConfig)
	}
}
func (this *Module) Initialize() {
	if this.initialized {
		return
	}

	// 如果没有配置任何连接时，默认一个
	if len(this.configs) == 0 {
		this.configs[infra.DEFAULT] = Config{
			Driver: infra.DEFAULT, Weight: 1, Codec: infra.GOB,
		}
	} else {
		// 默认分布， 如果想不参与分布，Weight设置为小于0 即可
		for key, config := range this.configs {
			if config.Weight == 0 {
				config.Weight = 1
			}
			if config.External {
				config.Weight = -1
			}
			this.configs[key] = config
		}

	}

	//拦截器
	this.serveFilters = make([]ctxFunc, 0)
	this.requestFilters = make([]ctxFunc, 0)
	this.executeFilters = make([]ctxFunc, 0)
	this.responseFilters = make([]ctxFunc, 0)
	for _, filter := range this.filters {
		if filter.Serve != nil {
			this.serveFilters = append(this.serveFilters, filter.Serve)
		}
		if filter.Request != nil {
			this.requestFilters = append(this.requestFilters, filter.Request)
		}
		if filter.Execute != nil {
			this.executeFilters = append(this.executeFilters, filter.Execute)
		}
		if filter.Response != nil {
			this.responseFilters = append(this.responseFilters, filter.Response)
		}
	}

	//处理器
	this.foundHandlers = make([]ctxFunc, 0)
	this.errorHandlers = make([]ctxFunc, 0)
	this.failedHandlers = make([]ctxFunc, 0)
	this.deniedHandlers = make([]ctxFunc, 0)
	for _, filter := range this.handlers {
		if filter.Found != nil {
			this.foundHandlers = append(this.foundHandlers, filter.Found)
		}
		if filter.Error != nil {
			this.errorHandlers = append(this.errorHandlers, filter.Error)
		}
		if filter.Failed != nil {
			this.failedHandlers = append(this.failedHandlers, filter.Failed)
		}
		if filter.Denied != nil {
			this.deniedHandlers = append(this.deniedHandlers, filter.Denied)
		}
	}

	for name, config := range this.events {

		// 分组设置
		// if config.Grouping && config.Group == "" {
		// 	config.Group = infra.Role()
		// }
		if config.Group {
			config.groupName = infra.Role()
			if config.groupName == "" {
				config.groupName = infra.INFRA //没有角色，全局统一，只有一个节点能收到
			}
		} else {
			//不分组的，每一个都不同，每一个节点都要收到消息
			config.groupName = infra.Generate()
		}

		this.events[name] = config
	}

	this.initialized = true
}
func (this *Module) Connect() {
	if this.connected {
		return
	}

	//协程池
	pool, err := ants.NewPool(-1)
	if err != nil {
		panic("Invalid event pool")
	}
	this.pool = pool

	//记录要参与分布的连接和权重
	weights := make(map[string]int)

	for name, config := range this.configs {
		driver, ok := this.drivers[config.Driver]
		if ok == false {
			panic("Invalid event driver: " + config.Driver)
		}

		inst := &Instance{
			this, name, config, nil,
		}

		// 建立连接
		connect, err := driver.Connect(inst)
		if err != nil {
			panic("Failed to connect to event: " + err.Error())
		}

		// 打开连接
		err = connect.Open()
		if err != nil {
			panic("Failed to open event connect: " + err.Error())
		}

		//注册，只有参与分布的才注册
		//不参与的都是外部的
		for msgName, msgConfig := range this.events {
			if msgConfig.Connect == "" || msgConfig.Connect == "*" || msgConfig.Connect == name {
				// 分组时，每一个订阅的group必须统一为同一个
				realName := config.Prefix + msgName
				realGroup := msgConfig.groupName

				// 注册队列
				if err := connect.Register(realName, realGroup); err != nil {
					panic("Failed to register event: " + err.Error())
				}
			}
		}

		inst.connect = connect

		//保存实例
		this.instances[name] = inst

		//只有设置了权重的才参与分布
		if config.Weight > 0 {
			weights[name] = config.Weight
		}
	}

	//hashring分片
	this.weights = weights
	this.hashring = util.NewHashRing(weights)

	this.connected = true
}
func (this *Module) Launch() {
	if this.launched {
		return
	}

	//全部开始来来来
	for _, inst := range this.instances {
		err := inst.connect.Start()
		if err != nil {
			panic("Failed to start event: " + err.Error())
		}
	}

	this.launched = true
}
func (this *Module) Terminate() {
	// 先停止订阅，不再接受新消息
	for _, ins := range this.instances {
		ins.connect.Stop()
	}

	//关闭协程池
	this.pool.ReleaseTimeout(time.Minute)

	//关闭所有连接
	for _, ins := range this.instances {
		ins.connect.Close()
	}

	this.launched = false
	this.connected = false
	this.initialized = false
}
