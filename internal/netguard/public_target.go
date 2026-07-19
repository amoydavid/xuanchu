// Package netguard 提供出站公网请求的共享 SSRF 防护。
//
// 它只负责 DNS 解析、IP 网段判断和公网地址选择，不绑定 webhook、
// notification 或附件任何业务语义。Hook、Notification、远程图片抓取
// 都应通过本包获得一致的私网/保留地址拦截。
package netguard

import (
	"context"
	"errors"
	"net"
)

// Resolver 抽象 DNS 解析，便于测试中注入 mock。
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// DefaultResolver 返回基于 net.DefaultResolver 的解析器。
func DefaultResolver() Resolver { return defaultResolverInstance }

type defaultResolver struct{}

var defaultResolverInstance Resolver = defaultResolver{}

func (defaultResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// ErrHostBlocked 表示主机解析结果包含禁止地址。
var ErrHostBlocked = errors.New("netguard: host resolves to blocked address")

// ErrNoAddresses 表示主机解析得到空地址集合。
var ErrNoAddresses = errors.New("netguard: host resolved to no addresses")

// IsBlockedIP 判断给定 IP 是否属于禁止出站访问的网段。
//
// 当前禁止：loopback、link-local unicast/multicast、multicast、unspecified、
// Go 标准库认定的 private（含 RFC1918 与 IPv6 ULA），以及运营商级 NAT
// (RFC 6598, 100.64.0.0/10)。
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	if ip.IsPrivate() {
		return true
	}
	// IPv4-mapped IPv6 也要归一化后再判 RFC6598。
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
	}
	return false
}

// ResolvePublic 解析 host 并要求所有结果都是公网地址。
//
// 返回原始解析结果（可能多条），调用方应自行使用其中之一连接。
// 任一地址被 IsBlockedIP 命中即整次失败；空结果返回 ErrNoAddresses。
func ResolvePublic(ctx context.Context, resolver Resolver, host string) ([]net.IPAddr, error) {
	if resolver == nil {
		resolver = DefaultResolver()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, ErrNoAddresses
	}
	for _, addr := range addrs {
		if IsBlockedIP(addr.IP) {
			return nil, ErrHostBlocked
		}
	}
	return addrs, nil
}
