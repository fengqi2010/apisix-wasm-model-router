package main

import (
	"github.com/tidwall/gjson"
	"regexp"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

type vmContext struct {
	types.DefaultVMContext
}

func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

type pluginContext struct {
	types.DefaultPluginContext
	config pluginConfig
}

type pluginConfig struct {
	contentType []string
	path        *regexp.Regexp
	field       string
}

func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &modelRouter{
		config: p.config,
	}
}

func (p *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	proxywasm.LogInfo("loading plugin config")
	data, err := proxywasm.GetPluginConfiguration()
	if data == nil {
		return types.OnPluginStartStatusOK
	}

	if err != nil {
		proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	if !gjson.Valid(string(data)) {
		proxywasm.LogCritical(`invalid configuration format; expected {"header": "<header name>", "value": "<header value>"}`)
		return types.OnPluginStartStatusFailed
	}

	result := gjson.ParseBytes(data)
	array := result.Get("contentType").Array()
	contentType := make([]string, len(array))
	for _, v := range array {
		contentType = append(contentType, v.String())
	}
	p.config = pluginConfig{
		contentType: contentType,
		path:        regexp.MustCompile(result.Get("path").String()),
		field:       result.Get("field").String(),
	}
	proxywasm.LogInfo("loading plugin config done")
	return types.OnPluginStartStatusOK
}

type modelRouter struct {
	types.DefaultHttpContext

	config pluginConfig
}

func (ctx *modelRouter) OnHttpRequestHeaders(_ int, _ bool) types.Action {
	// https://apisix.apache.org/zh/docs/apisix/wasm/
	err := proxywasm.SetProperty([]string{"wasm_process_req_body"}, []byte("true"))
	if err != nil {
		return types.ActionContinue
	}
	return types.ActionContinue
}

func (ctx *modelRouter) OnHttpRequestBody(bodySize int, endOfStream bool) types.Action {
	if !endOfStream {
		// Wait until we see the entire body to replace.
		return types.ActionPause
	}
	// 获取请求的content-type
	contentType, err := proxywasm.GetHttpRequestHeader("content-type")
	if err != nil {
		// 不符合content-type
		proxywasm.LogInfof("error getting content type: %v", err)
		return types.ActionContinue
	}
	found := false
	for _, s := range ctx.config.contentType {
		if s == contentType {
			found = true // 符合content-type
		}
	}
	if !found {
		proxywasm.LogInfof("content type %s not allowed", contentType)
		return types.ActionContinue
	}
	// 获取请求的path
	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil || !ctx.config.path.MatchString(path) {
		// 不符合匹配规则
		proxywasm.LogInfof("path %s not allowed", path)
		return types.ActionContinue
	}
	originalBody, err := proxywasm.GetHttpRequestBody(0, bodySize)
	if err != nil {
		proxywasm.LogInfof("error getting original body: %v", err)
		return types.ActionContinue
	}
	field := gjson.Get(string(originalBody), ctx.config.field)
	if !field.Exists() {
		proxywasm.LogInfof("field %s not found", ctx.config.field)
		return types.ActionContinue
	}
	headerName := "x-" + ctx.config.field + "-router"
	headerValue := field.String()
	err = proxywasm.AddHttpRequestHeader(headerName, headerValue)
	if err != nil {
		proxywasm.LogInfof("failed to %s request header: %v", headerValue, err)
		return types.ActionContinue
	}
	proxywasm.LogDebugf("request header added [{%s: %s}]", headerName, headerValue)
	return types.ActionContinue
}

func (ctx *modelRouter) OnHttpResponseHeaders(_ int, endOfStream bool) types.Action {
	if !endOfStream {
		return types.ActionPause
	}
	err := proxywasm.AddHttpResponseHeader("x-"+ctx.config.field+"-router-by", "wasm")
	if err != nil {
		return types.ActionContinue
	}
	return types.ActionContinue
}
