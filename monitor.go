package event

import (
	"github.com/infrago/base"
	"github.com/infrago/infra"
)

func (m *Module) Ready() bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.started && len(m.instances) > 0
}

func (m *Module) Health() infra.ModuleHealth {
	m.mutex.RLock()
	started := m.started
	connections := len(m.instances)
	events := len(m.events)
	m.mutex.RUnlock()
	return infra.NewModuleHealth("event", started && connections > 0, nil, base.Map{
		"connections": connections,
		"events":      events,
	})
}

func (m *Module) Stats() infra.ModuleStats {
	m.mutex.RLock()
	started := m.started
	connections := len(m.instances)
	events := len(m.events)
	declares := len(m.declares)
	m.mutex.RUnlock()
	return infra.NewModuleStats("event", started && connections > 0, base.Map{
		"connections": connections,
		"events":      events,
		"declares":    declares,
	})
}
