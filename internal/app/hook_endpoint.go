package app

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"
)

// HookHostResolver 抽象 DNS 解析，便于测试中注入 mock。
type HookHostResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

type defaultResolver struct{}

func (defaultResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// DefaultHookResolver 返回默认的 DNS 解析器。
func DefaultHookResolver() HookHostResolver { return defaultHookResolver }

var defaultHookResolver HookHostResolver = defaultResolver{}

// ValidateWebhookEndpointURL 校验 webhook 目标 URL，防止 SSRF。
func ValidateWebhookEndpointURL(ctx context.Context, raw string, resolver HookHostResolver) error {
	if raw == "" {
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "endpoint URL is required"}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "invalid URL: " + err.Error()}
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "only http and https schemes are allowed"}
	}
	host := parsed.Hostname()
	if host == "" {
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "host is required"}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "cannot resolve host: " + err.Error()}
	}
	if len(addrs) == 0 {
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "host resolved to no addresses"}
	}
	for _, addr := range addrs {
		if isBlockedIP(addr.IP) {
			return RuntimeError{Code: "hook_endpoint_invalid", Message: "endpoint resolves to blocked address"}
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	// RFC 6598 运营商级 NAT (100.64.0.0/10)
	if ip.To4() != nil {
		ip4 := ip.To4()
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

// IsBlockedWebhookIP reports whether an IP address is unsafe for outbound webhook delivery.
func IsBlockedWebhookIP(ip net.IP) bool {
	return isBlockedIP(ip)
}

// ValidateWebhookEndpointURLWithDefault 使用默认 DNS 解析器校验 webhook URL。
func ValidateWebhookEndpointURLWithDefault(raw string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ValidateWebhookEndpointURL(ctx, raw, defaultHookResolver)
}
