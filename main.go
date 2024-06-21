// This WASM plugin for Envoy is intended to keep only the first IP from x-forwarded-for hops chain.
// Ideally, it should be used on Istio sidecars' 'AUTHZ' filter-chain phase.
// Its mission is cleaning Xff header before using an AuthorizationPolicy to limit origins for the requests,
// as it only works with rightmost IP in mentioned header.
// This plugin also sets 'x-original-forwarded-for' header with original chain not to lose critical information.

// Ref: https://medium.com/trendyol-tech/extending-envoy-proxy-wasm-filter-with-golang-9080017f28ea
// Ref: https://github.com/tetratelabs/proxy-wasm-go-sdk/blob/main/examples/http_headers/

package main

import (
	"strings"

	"github.com/tidwall/gjson"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

const (
	HttpHeaderXff         = "x-forwarded-for"
	HttpHeaderOriginalXff = "x-original-forwarded-for"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

// vmContext implements types.VMContext.
type vmContext struct {
	// Embed the default VM context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultVMContext
}

// NewPluginContext implements types.VMContext.
func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

// pluginContext implements types.PluginContext.
type pluginContext struct {
	// Embed the default plugin context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultPluginContext

	// headerName and headerValue are the header to be added to response. They are configured via
	// plugin configuration during OnPluginStart.
	headerName  string
	headerValue string
}

// NewHttpContext implements types.PluginContext.
func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpHeaders{
		contextID:   contextID,
		headerName:  p.headerName,
		headerValue: p.headerValue,
	}
}

// OnPluginStart implements types.PluginContext.
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

	p.headerName = strings.TrimSpace(gjson.Get(string(data), "header").Str)
	p.headerValue = strings.TrimSpace(gjson.Get(string(data), "value").Str)

	if p.headerName == "" || p.headerValue == "" {
		proxywasm.LogCritical(`invalid configuration format; expected {"header": "<header name>", "value": "<header value>"}`)
		return types.OnPluginStartStatusFailed
	}

	proxywasm.LogInfof("header from config: %s = %s", p.headerName, p.headerValue)

	return types.OnPluginStartStatusOK
}

// httpHeaders implements types.HttpContext.
type httpHeaders struct {
	// Embed the default http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext
	contextID   uint32
	headerName  string
	headerValue string
}

// OnHttpRequestHeaders implements types.HttpContext.
func (ctx *httpHeaders) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {

	hs, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		proxywasm.LogCriticalf("failed to get request headers: %v", err)
	}

	// Loop over the headers to look for ours
	for _, h := range hs {
		if h[0] != HttpHeaderXff {
			continue
		}

		// Preserve Xff data into an alternative header
		err := proxywasm.AddHttpRequestHeader(HttpHeaderOriginalXff, h[1])
		if err != nil {
			proxywasm.LogCriticalf("failed to set '%s' header: %v", HttpHeaderOriginalXff, err)
		}

		// Replace Xff header
		sourceHops := strings.Split(h[1], ",")
		proxywasm.LogInfof("original client ip found: %s", sourceHops[0])

		err = proxywasm.ReplaceHttpRequestHeader(HttpHeaderXff, sourceHops[0])
		if err != nil {
			proxywasm.LogCriticalf("failed to replace '%s' header: %v", HttpHeaderXff, err)
		}

		break
	}
	return types.ActionContinue
}
