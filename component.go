package event

import (
	. "github.com/infrago/base"
	"github.com/infrago/infra"
)

type (
	Event struct {
		Name     string   `json:"name"`
		Text     string   `json:"text"`
		Alias    []string `json:"alias"`
		Nullable bool     `json:"-"`
		Args     Vars     `json:"args"`
		Setting  Map      `json:"-"`
		Coding   bool     `json:"-"`

		Action  ctxFunc   `json:"-"`
		Actions []ctxFunc `json:"-"`

		// 路由单独可定义的处理器
		Found  ctxFunc `json:"-"`
		Error  ctxFunc `json:"-"`
		Failed ctxFunc `json:"-"`
		Denied ctxFunc `json:"-"`

		Connect   string `json:"connect"`
		Group     bool   `json:"group"`
		groupName string `json:"-"`
	}

	// Notice 通知，表示当前节点会发出的事件预告
	// 比如，支付模块，可能会发布 pay.Success 之类的一系列的支付完成的事件
	// 在集群模式下，应该会把节点的notice写入集群节点信息下
	// 这样方便，生成分布式的文档，知道哪些节点会发布哪些通知出来
	Notice struct {
		Name     string `json:"name"`
		Text     string `json:"text"`
		Nullable bool   `json:"-"`
		Args     Vars   `json:"args"`
	}

	// Filter 拦截器
	Filter struct {
		Name     string  `json:"name"`
		Text     string  `json:"text"`
		Serve    ctxFunc `json:"-"`
		Request  ctxFunc `json:"-"`
		Execute  ctxFunc `json:"-"`
		Response ctxFunc `json:"-"`
	}
	// Handler 处理器
	Handler struct {
		Name   string  `json:"name"`
		Text   string  `json:"text"`
		Found  ctxFunc `json:"-"`
		Error  ctxFunc `json:"-"`
		Failed ctxFunc `json:"-"`
		Denied ctxFunc `json:"-"`
	}
)

func (module *Module) Event(name string, config Event) {
	module.mutex.Lock()
	defer module.mutex.Unlock()

	alias := make([]string, 0)
	if name != "" {
		alias = append(alias, name)
	}
	if config.Alias != nil {
		alias = append(alias, config.Alias...)
	}

	for _, key := range alias {
		if infra.Override() {
			module.events[key] = config
		} else {
			if _, ok := module.events[key]; ok == false {
				module.events[key] = config
			}
		}
	}
}

// Notice 注册 预告
func (module *Module) Notice(name string, config Notice) {
	if infra.Override() {
		module.notices[name] = config
	} else {
		if _, ok := module.notices[name]; ok == false {
			module.notices[name] = config
		}
	}
}

// Filter 注册 拦截器
func (module *Module) Filter(name string, config Filter) {
	if infra.Override() {
		module.filters[name] = config
	} else {
		if _, ok := module.filters[name]; ok == false {
			module.filters[name] = config
		}
	}
}

// Handler 注册 处理器
func (module *Module) Handler(name string, config Handler) {
	if infra.Override() {
		module.handlers[name] = config
	} else {
		if _, ok := module.handlers[name]; ok == false {
			module.handlers[name] = config
		}
	}
}
