package event

import (
	. "github.com/infrago/base"
	"github.com/infrago/infra"
)

type (
	Context struct {
		inst *Instance
		*infra.Meta

		index int
		nexts []ctxFunc

		Name    string
		Config  *Event
		Setting Map

		Value  Map
		Args   Map
		Locals Map
		Body   Any
	}

	ctxFunc func(*Context)
)

func (ctx *Context) clear() {
	ctx.index = 0
	ctx.nexts = make([]ctxFunc, 0)
}

func (ctx *Context) next(nexts ...ctxFunc) {
	ctx.nexts = append(ctx.nexts, nexts...)
}

func (ctx *Context) Next() {
	if len(ctx.nexts) > ctx.index {
		next := ctx.nexts[ctx.index]
		ctx.index++
		if next != nil {
			next(ctx)
		} else {
			ctx.Next()
		}
	}
}

func (ctx *Context) Found() {
	ctx.inst.found(ctx)
}

func (ctx *Context) Error(res Res) {
	ctx.Result(res)
	ctx.inst.error(ctx)
}

func (ctx *Context) Failed(res Res) {
	ctx.Result(res)
	ctx.inst.failed(ctx)
}

func (ctx *Context) Retry(res Res) {
	ctx.Result(infra.RetryResult(res))
	ctx.inst.failed(ctx)
}

func (ctx *Context) Denied(res Res) {
	ctx.Result(res)
	ctx.inst.denied(ctx)
}

func (ctx *Context) Publish(name string, values ...Map) error {
	return module.publishMeta(ctx.Meta, "", name, values...)
}

func (ctx *Context) PublishTo(conn, name string, values ...Map) error {
	return module.publishMeta(ctx.Meta, conn, name, values...)
}

func (ctx *Context) Broadcast(name string, values ...Map) error {
	return module.broadcastMeta(ctx.Meta, "", name, values...)
}

func (ctx *Context) BroadcastTo(conn, name string, values ...Map) error {
	return module.broadcastMeta(ctx.Meta, conn, name, values...)
}
