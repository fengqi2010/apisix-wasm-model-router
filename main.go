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
	contentType string
	path        *regexp.Regexp
	field       string
}

func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &parseBodyToHeader{
		config: p.config,
	}
}

func (p *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	proxywasm.LogDebug("loading plugin config")
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
	p.config = pluginConfig{
		contentType: result.Get("content-type").String(),
		path:        regexp.MustCompile(result.Get("path").String()),
		field:       result.Get("field").String(),
	}

	return types.OnPluginStartStatusOK
}

type parseBodyToHeader struct {
	types.DefaultHttpContext

	config          pluginConfig
	modifyResponse  bool
	bufferOperation string
}

func (ctx *parseBodyToHeader) OnHttpRequestBody(bodySize int, endOfStream bool) types.Action {
	if ctx.modifyResponse {
		return types.ActionContinue
	}
	if !endOfStream {
		// Wait until we see the entire body to replace.
		return types.ActionPause
	}
	// 获取请求的content-type
	contentType, err := proxywasm.GetHttpRequestHeader("content-type")
	if err != nil || contentType != ctx.config.contentType {
		// 不符合content-type
		return types.ActionContinue
	}
	// 获取请求的path
	path, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil || !ctx.config.path.MatchString(path) {
		// 不符合匹配规则
		return types.ActionContinue
	}
	originalBody, err := proxywasm.GetHttpRequestBody(0, bodySize)
	if err != nil {
		proxywasm.LogErrorf("failed to get request body: %v", err)
		return types.ActionContinue
	}
	model := gjson.Get(string(originalBody), ctx.config.field)
	if !model.Exists() {
		return types.ActionContinue
	}
	err = proxywasm.AddHttpRequestHeader(":proxy-model", model.String())
	if err != nil {
		proxywasm.LogErrorf("failed to %s request body: %v", ctx.bufferOperation, err)
		return types.ActionContinue
	}
	return types.ActionContinue
}
