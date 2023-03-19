package event

import (
	"strings"

	. "github.com/infrago/base"
	"github.com/infrago/infra"
)

func (this *Instance) Submit(next func()) {
	this.module.pool.Submit(next)
}

func (this *Instance) newContext() *Context {
	return &Context{
		inst: this, index: 0, nexts: make([]ctxFunc, 0),
		Setting: Map{}, Value: Map{}, Args: Map{}, Locals: Map{},
	}
}

// 收到消息
// 加入协助程池
func (this *Instance) Serve(name string, data []byte) {
	//事件不限制线程数，所有消息都要接收和处理
	this.module.pool.Submit(func() {
		this.serving(name, data)
	})
}

// parse 解析元数据
// 加入payload直解
func (this *Instance) parse(data []byte) (infra.Metadata, error) {
	metadata := infra.Metadata{}
	err := infra.Unmarshal(this.Config.Codec, data, &metadata)
	if err != nil {
		//原始数据结构，直接当Payload
		payload := Map{}
		err = infra.Unmarshal(this.Config.Codec, data, &payload)
		if err == nil {
			metadata.Payload = payload
		}
	}

	return metadata, nil
}

// 收到消息
// 待优化，加入协程池，限制单个事件的并发
func (this *Instance) serving(name string, data []byte) {
	if strings.HasPrefix(name, this.Config.Prefix) {
		name = strings.TrimPrefix(name, this.Config.Prefix)
	}

	ctx := this.newContext()
	if cfg, ok := this.module.events[name]; ok {
		ctx.Name = name
		ctx.Config = &cfg
		ctx.Setting = cfg.Setting
	}

	// 解析元数据
	metadata, err := this.parse(data)
	if err == nil {
		ctx.Metadata(metadata)
		ctx.Value = metadata.Payload
	}

	//开始执行
	this.open(ctx)
	infra.CloseMeta(&ctx.Meta)
	this.close(ctx)
}

// ctx 收尾工作
func (this *Instance) close(ctx *Context) {
}

func (this *Instance) open(ctx *Context) {
	//清理执行线
	ctx.clear()

	//serve拦截器
	ctx.next(this.module.serveFilters...)
	ctx.next(this.serve)

	//开始执行
	ctx.Next()
}
func (this *Instance) serve(ctx *Context) {
	//清理执行线
	ctx.clear()

	//request拦截器
	ctx.next(this.module.requestFilters...)
	ctx.next(this.request)

	//开始执行
	ctx.Next()

	//response得在这里
	this.response(ctx)
}

// request 请求处理
func (this *Instance) request(ctx *Context) {
	ctx.clear()

	//request拦截器
	ctx.next(this.finding)     //存在否
	ctx.next(this.authorizing) //身份验证
	ctx.next(this.arguing)     //参数处理
	ctx.next(this.execute)

	//开始执行
	ctx.Next()
}

// execute 执行线
func (this *Instance) execute(ctx *Context) {
	ctx.clear()

	//execute拦截器
	ctx.next(this.module.executeFilters...)
	if ctx.Config.Actions != nil || len(ctx.Config.Actions) > 0 {
		ctx.next(ctx.Config.Actions...)
	}
	if ctx.Config.Action != nil {
		ctx.next(ctx.Config.Action)
	}

	//开始执行
	ctx.Next()
}

// response 响应线
func (this *Instance) response(ctx *Context) {
	ctx.clear()

	//response拦截器
	ctx.next(this.module.responseFilters...)

	//开始执行
	ctx.Next()

	//这样保证body一定会执行，要不然response不next就没法弄了
	this.body(ctx)
}

// finding 判断不
func (this *Instance) finding(ctx *Context) {
	if ctx.Config == nil {
		ctx.Found()
	} else {
		ctx.Next()
	}
}

// authorizing token验证
func (this *Instance) authorizing(ctx *Context) {
	// 待处理
	ctx.Next()
}

// arguing 参数解析
func (this *Instance) arguing(ctx *Context) {
	if ctx.Config.Args != nil {
		argsValue := Map{}
		res := infra.Mapping(ctx.Config.Args, ctx.Value, argsValue, ctx.Config.Nullable, false, ctx.Timezone())
		if res != nil && res.Fail() {
			ctx.Failed(res)
		}

		for k, v := range argsValue {
			ctx.Args[k] = v
		}
	}
	ctx.Next()
}

func (this *Instance) found(ctx *Context) {
	ctx.clear()

	//把处理器加入调用列表
	if ctx.Config.Found != nil {
		ctx.next(ctx.Config.Found)
	}
	ctx.next(this.module.foundHandlers...)

	ctx.Next()
}

func (this *Instance) error(ctx *Context) {
	ctx.clear()

	//把处理器加入调用列表
	if ctx.Config.Error != nil {
		ctx.next(ctx.Config.Error)
	}
	ctx.next(this.module.errorHandlers...)

	ctx.Next()
}

func (this *Instance) failed(ctx *Context) {
	ctx.clear()

	//把处理器加入调用列表
	if ctx.Config.Failed != nil {
		ctx.next(ctx.Config.Failed)
	}
	ctx.next(this.module.failedHandlers...)

	ctx.Next()
}

func (this *Instance) denied(ctx *Context) {
	ctx.clear()

	//把处理器加入调用列表
	if ctx.Config.Denied != nil {
		ctx.next(ctx.Config.Denied)
	}
	ctx.next(this.module.deniedHandlers...)

	ctx.Next()
}

// 最终的默认body响应
// 其实 event 目前没有任何需要反馈的
func (this *Instance) body(ctx *Context) {
	// switch body := ctx.Body.(type) {
	// default:
	// 	this.bodyDefault(ctx)
	// }
	this.bodyDefault(ctx)
}

// bodyDefault 默认的body处理
func (this *Instance) bodyDefault(ctx *Context) {
	//here is nothing todo
}
