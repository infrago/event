package event

import (
	. "github.com/infrago/base"
)

type (
	// Driver 数据驱动
	Driver interface {
		Connect(*Instance) (Connect, error)
	}
	Health struct {
		Workload int64
	}

	// Connect 连接
	Connect interface {
		Open() error
		Health() (Health, error)
		Close() error

		Register(name, group string) error

		Start() error
		Stop() error

		Publish(name string, data []byte) error
	}

	Instance struct {
		connect Connect
		Name    string
		Config  Config
		Setting Map
	}
)
