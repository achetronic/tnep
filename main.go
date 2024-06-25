// This WASM plugin for Envoy is designed to manage non-trusted IPs in the 'x-forwarded-for' HTTP header.
// Ideally, it should be used in the 'AUTHZ' filter chain phase of Istio sidecars.

// Its purpose is to sanitize the XFF header before applying an AuthorizationPolicy to restrict origins for requests,
// as this policy only operates on the rightmost IP in the mentioned header.
// Additionally, this plugin sets the 'x-original-forwarded-for' header with the original chain to preserve critical information.

// Ref: https://github.com/tetratelabs/proxy-wasm-go-sdk/blob/main/examples/http_headers/

package main

import (
	"github.com/tidwall/gjson"
	"net"
	"strings"

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

	// Following fields are configured via plugin configuration during OnPluginStart.

	// trustedNetworks are the CIDRs from where we check XFF IPs during a request.
	trustedNetworks []*net.IPNet
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
		proxywasm.LogCritical(`invalid configuration format; expected {"trusted_networks": ["<cidr>"...]}`)
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
}

// NewHttpContext implements types.PluginContext.
func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpHeaders{
		contextID: contextID,

		// TODO
		trustedNetworks: p.trustedNetworks,
	}
}

// OnHttpRequestHeaders implements types.HttpContext.
func (ctx *httpHeaders) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {

	requestHeaders, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		proxywasm.LogCriticalf("failed to get request headers: %v", err)
	}

	// Loop over the headers to look for ours
	for _, requestHeader := range requestHeaders {
		if requestHeader[0] != HttpHeaderXff {
			continue
		}

		var resultingSourceHops []string

		// 88.x.x.x,34.y.y.y,35.z.z.z,10.a.a.a -> [88.x.x.x, 34.y.y.y, 35.z.z.z, 10.a.a.a]
		sourceHopsRaw := strings.Split(requestHeader[1], ",")

		// Look for the IPs into the CIDRs
		for _, sourceHopRaw := range sourceHopsRaw {

			sourceHop := net.ParseIP(sourceHopRaw)

			// Check if is current processed IP is trusted according to configured CIDRs
			if !isTrustedIp(ctx.trustedNetworks, sourceHop) {
				resultingSourceHops = append(resultingSourceHops, sourceHop.String())
			}
		}

		// Preserve Xff data into an alternative header
		err := proxywasm.AddHttpRequestHeader(HttpHeaderOriginalXff, requestHeader[1])
		if err != nil {
			proxywasm.LogCriticalf("failed to set '%s' header: %v", HttpHeaderOriginalXff, err)
		}

		// Replace Xff header
		err = proxywasm.ReplaceHttpRequestHeader(HttpHeaderXff, strings.Join(resultingSourceHops, ","))
		if err != nil {
			proxywasm.LogCriticalf("failed to replace '%s' header: %v", HttpHeaderXff, err)
		}

		break
	}
	return types.ActionContinue
}
