package app

import (
	"context"
	"net"
	"net/url"
	"strings"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/netguard"
)

// HookHostResolver 抽象 DNS 解析，便于测试中注入 mock。
//
// 历史别名：保持与 netguard.Resolver 同义，方便既有调用方迁移。
type HookHostResolver = netguard.Resolver

// DefaultHookResolver 返回默认的 DNS 解析器。
func DefaultHookResolver() HookHostResolver { return defaultHookResolver }

var defaultHookResolver HookHostResolver = netguard.DefaultResolver()

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
	_, err = netguard.ResolvePublic(ctx, resolver, host)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return RuntimeError{Code: "hook_endpoint_invalid", Message: "endpoint address rejected: " + err.Error()}
	}
	return nil
}

// IsBlockedWebhookIP 历史导出别名，委托给 netguard。
func IsBlockedWebhookIP(ip net.IP) bool {
	return netguard.IsBlockedIP(ip)
}

// ValidateWebhookEndpointURLWithDefault 使用默认 DNS 解析器校验 webhook URL。
func ValidateWebhookEndpointURLWithDefault(raw string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return ValidateWebhookEndpointURL(ctx, raw, defaultHookResolver)
}
