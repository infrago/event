package event

import (
	. "github.com/infrago/base"
	"github.com/infrago/infra"
)

type (
	Event struct {
		Alias    []string `json:"alias"`
		Name     string   `json:"name"`
		Desc     string   `json:"desc"`
		Nullable bool     `json:"-"`
		Args     Vars     `json:"args"`
		Setting  Map      `json:"setting"`

		Action  ctxFunc   `json:"-"`
		Actions []ctxFunc `json:"-"`

		Found  ctxFunc `json:"-"`
		Error  ctxFunc `json:"-"`
		Failed ctxFunc `json:"-"`
		Denied ctxFunc `json:"-"`

		Connect string `json:"connect"`
	}

	Events map[string]Event

	Declare struct {
		Alias    []string `json:"alias"`
		Name     string   `json:"name"`
		Desc     string   `json:"desc"`
		Nullable bool     `json:"-"`
		Args     Vars     `json:"args"`
	}

	Filter struct {
		Name     string  `json:"name"`
		Desc     string  `json:"desc"`
		Serve    ctxFunc `json:"-"`
		Request  ctxFunc `json:"-"`
		Execute  ctxFunc `json:"-"`
		Response ctxFunc `json:"-"`
	}

	Handler struct {
		Name   string  `json:"name"`
		Desc   string  `json:"desc"`
		Found  ctxFunc `json:"-"`
		Error  ctxFunc `json:"-"`
		Failed ctxFunc `json:"-"`
		Denied ctxFunc `json:"-"`
	}
)

func (m *Module) RegisterEvent(name string, cfg Event) {
	keys := collectAlias(name, cfg.Alias)
	declare := Declare{
		Alias:    cfg.Alias,
		Name:     cfg.Name,
		Desc:     cfg.Desc,
		Nullable: cfg.Nullable,
		Args:     cfg.Args,
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.opened || m.started {
		return
	}
	for _, key := range keys {
		if validateEventName(key) != nil {
			continue
		}
		if infra.Override() {
			m.events[key] = cfg
		} else if _, ok := m.events[key]; !ok {
			m.events[key] = cfg
		}
	}
	for _, key := range keys {
		if validateEventName(key) != nil {
			continue
		}
		if infra.Override() {
			m.declares[key] = declare
		} else if _, ok := m.declares[key]; !ok {
			m.declares[key] = declare
		}
	}
}

func (m *Module) RegisterDeclare(name string, cfg Declare) {
	keys := collectAlias(name, cfg.Alias)

	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.opened || m.started {
		return
	}
	for _, key := range keys {
		if validateEventName(key) != nil {
			continue
		}
		if infra.Override() {
			m.declares[key] = cfg
		} else if _, ok := m.declares[key]; !ok {
			m.declares[key] = cfg
		}
	}
}

func (m *Module) RegisterFilter(name string, cfg Filter) {
	if name == "" {
		return
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.opened || m.started {
		return
	}
	if infra.Override() {
		m.filters[name] = cfg
	} else if _, ok := m.filters[name]; !ok {
		m.filters[name] = cfg
	}
}

func (m *Module) RegisterHandler(name string, cfg Handler) {
	if name == "" {
		return
	}
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if m.opened || m.started {
		return
	}
	if infra.Override() {
		m.handlers[name] = cfg
	} else if _, ok := m.handlers[name]; !ok {
		m.handlers[name] = cfg
	}
}

func collectAlias(name string, alias []string) []string {
	keys := make([]string, 0, 1+len(alias))
	if name != "" {
		keys = append(keys, name)
	}
	keys = append(keys, alias...)
	return keys
}
