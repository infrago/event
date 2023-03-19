package event

import (
	. "github.com/infrago/base"
)

// Publish 发起消息
func (this *Module) Notify(name string, values ...Map) error {
	return this.notify("", name, values...)
}

// notify 指定连接发起消息
func (this *Module) NotifyTo(conn, name string, values ...Map) error {
	return this.notify(conn, name, values...)
}
