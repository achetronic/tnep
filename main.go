// This WASM plugin for Envoy is designed to manage non-trusted IPs in the 'x-forwarded-for' HTTP header.
// Ideally, it should be used in the 'AUTHZ' filter chain phase of Istio sidecars.

// Its purpose is to sanitize the XFF header before applying an AuthorizationPolicy to restrict origins for requests,
// as this policy only operates on the rightmost IP in the mentioned header.
// Additionally, this plugin sets the 'x-original-forwarded-for' header with the original chain to preserve critical information.

// Ref: https://github.com/tetratelabs/proxy-wasm-go-sdk/blob/main/examples/http_headers/

package main

import (
	"github.com/proxy-wasm/proxy-wasm-go-sdk/proxywasm"
	"github.com/proxy-wasm/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
	"net"
	"strings"
)

const (
	HttpHeaderXff = "x-forwarded-for"
)

func main() {
	//proxywasm.SetVMContext(&vmContext{})
}

func init() {
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

	// Following fields are configured via plugin configuration during OnPluginStart.

	// trustedNetworks are the CIDRs from where we check XFF IPs during a request.
	trustedNetworks []*net.IPNet

	// injectedHeaderName TODO
	injectedHeaderName string

	// overwriteHeaderOnExists TODO
	overwriteHeaderOnExists bool
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
		proxywasm.LogCritical(`invalid configuration format; expected {"trusted_networks": ["<cidr>"...], "injected_header_name": "x-sample", "overwrite_header_on_exists": <bool> }`)
		return types.OnPluginStartStatusFailed
	}

	// Parse networks from 'trusted_networks' param
	configTrustedNetwork := gjson.Get(string(data), "trusted_networks").Array()
	for _, trustedNetwork := range configTrustedNetwork {

		_, netPtr, err := net.ParseCIDR(trustedNetwork.Str)
		if err != nil {
			proxywasm.LogCriticalf(`impossible to parse cidr from config: %s`, err)
			return types.OnPluginStartStatusFailed
		}

		p.trustedNetworks = append(p.trustedNetworks, netPtr)
	}

	// Parse header name from 'injected_header_name'
	p.injectedHeaderName = gjson.Get(string(data), "injected_header_name").Str
	if p.injectedHeaderName == "" {
		proxywasm.LogCritical(`injected_header_name param can not be empty`)
		return types.OnPluginStartStatusFailed
	}

	// Parse header name from 'overwrite_header_on_exists'
	overwriteHeaderOnExistsRaw := gjson.Get(string(data), "overwrite_header_on_exists")
	if !overwriteHeaderOnExistsRaw.IsBool() {
		proxywasm.LogCritical(`overwrite_header_on_exists param must be boolean`)
		return types.OnPluginStartStatusFailed
	}

	p.overwriteHeaderOnExists = overwriteHeaderOnExistsRaw.Bool()

	//
	return types.OnPluginStartStatusOK
}

// httpHeaders implements types.HttpContext.
type httpHeaders struct {
	// Embed the default http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext
	contextID uint32

	// TODO
	trustedNetworks []*net.IPNet

	// injectedHeaderName TODO
	injectedHeaderName string

	// overwriteHeaderOnExists TODO
	overwriteHeaderOnExists bool
}

// NewHttpContext implements types.PluginContext.
func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpHeaders{
		contextID: contextID,

		// TODO
		trustedNetworks: p.trustedNetworks,

		// TODO
		injectedHeaderName: p.injectedHeaderName,

		// TODO
		overwriteHeaderOnExists: p.overwriteHeaderOnExists,
	}
}

// OnHttpRequestHeaders implements types.HttpContext.
func (ctx *httpHeaders) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {

	// 1. Process XFF header to find out 'realClientIp'
	requestHeaders, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		proxywasm.LogCriticalf("failed to get request headers: %v", err)
	}

	var sourceHopsRaw []string
	var realClientIp string

	// Loop over the headers to look for ours
	for _, requestHeader := range requestHeaders {
		if requestHeader[0] != HttpHeaderXff {
			continue
		}

		var resultingSourceHops []string

		// 88.x.x.x,34.y.y.y,35.z.z.z,10.a.a.a -> [96.r.r.r, 93.u.u.u, 34.y.y.y, 35.z.z.z, 10.a.a.a, 88.x.x.x, 34.y.y.y, 35.z.z.z, 10.a.a.a]
		sourceHopsRaw = strings.Split(requestHeader[1], ",")

		// Look for the IPs into the CIDRs
		for _, sourceHopRaw := range sourceHopsRaw {

			sourceHop := net.ParseIP(sourceHopRaw)

			// Check if is current processed IP is trusted according to configured CIDRs
			if !isTrustedIp(ctx.trustedNetworks, sourceHop) {
				resultingSourceHops = append(resultingSourceHops, sourceHop.String())
			}
		}

		// TODO
		if len(resultingSourceHops) >= 1 {
			realClientIp = resultingSourceHops[len(resultingSourceHops)-1]
		}

		break
	}

	// 2. Process Injected Header according to the configured conditions
	if realClientIp == "" {
		return types.ActionContinue
	}

	injectedHeaderValue, err := proxywasm.GetHttpRequestHeader(ctx.injectedHeaderName)
	if err != nil {
		proxywasm.LogCriticalf("failed to get current value for injected header: %v", ctx.injectedHeaderName)
	}

	// Already present and overwrite NOT requested
	if injectedHeaderValue != "" && !ctx.overwriteHeaderOnExists {
		proxywasm.LogCriticalf("header '%v' already present. overwritting is disabled by configuration", ctx.injectedHeaderName)
	}

	// Already present and overwrite IS requested
	if injectedHeaderValue != "" && ctx.overwriteHeaderOnExists {
		err = proxywasm.ReplaceHttpRequestHeader(ctx.injectedHeaderName, realClientIp)
		if err != nil {
			proxywasm.LogCriticalf("failed to overwrite '%s' header: %v", ctx.injectedHeaderName, err)
		}
	}

	// Header not present, add it
	if injectedHeaderValue == "" {
		err = proxywasm.AddHttpRequestHeader(ctx.injectedHeaderName, realClientIp)
		if err != nil {
			proxywasm.LogCriticalf("failed to set '%s' header: %v", ctx.injectedHeaderName, err)
		}
	}

	return types.ActionContinue
}
