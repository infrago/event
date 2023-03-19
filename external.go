package event

import (
	. "github.com/infrago/base"
)

// func Publish(name string, values ...Map) error {
// 	return module.Publish(name, values...)
// }

// func PublishTo(conn, name string, values ...Map) error {
// 	return module.PublishTo(conn, name, values...)
// }

func Notify(name string, values ...Map) error {
	return module.Notify(name, values...)
}

func NotifyTo(conn, name string, values ...Map) error {
	return module.NotifyTo(conn, name, values...)
}
