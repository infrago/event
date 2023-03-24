package event

import (
	. "github.com/infrago/base"
)

// Publish 发起消息
func (this *Module) Publish(name string, values ...Map) error {
	return this.publish("", name, values...)
}

// PublishTo 指定连接发起消息
func (this *Module) PublishTo(conn, name string, values ...Map) error {
	return this.publish(conn, name, values...)
}
