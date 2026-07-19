package netguard

import (
	"context"
	"errors"
	"net"
	"testing"
)

type stubResolver struct {
	addrs []net.IPAddr
	err   error
}

func (s stubResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) {
	return s.addrs, s.err
}

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{"loopback v4", "127.0.0.1", true},
		{"loopback v4 alt", "127.255.255.255", true},
		{"loopback v6", "::1", true},
		{"private v4 10", "10.0.0.1", true},
		{"private v4 172", "172.16.0.1", true},
		{"private v4 192", "192.168.1.1", true},
		{"link-local v4", "169.254.1.1", true},
		{"link-local v6", "fe80::1", true},
		{"cgnat", "100.64.0.1", true},
		{"cgnat upper", "100.127.255.255", true},
		{"cgnat below lower", "100.63.255.255", false},
		{"cgnat above upper", "100.128.0.0", false},
		{"multicast v4", "224.0.0.1", true},
		{"multicast v6", "ff02::1", true},
		{"unspecified v4", "0.0.0.0", true},
		{"unspecified v6", "::", true},
		{"ula v6", "fc00::1", true},
		{"ula v6 fd", "fd00::1", true},
		{"public v4", "93.184.216.34", false},
		{"public v6", "2606:2800:220:1:248:1893:25c8:1946", false},
		{"ipv4-mapped v6 loopback", "::ffff:127.0.0.1", true},
		{"ipv4-mapped v6 cgnat", "::ffff:100.64.0.1", true},
		{"ipv4-mapped v6 public", "::ffff:93.184.216.34", false},
		{"nil", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ip net.IP
			if tc.raw != "" {
				ip = net.ParseIP(tc.raw)
			}
			if got := IsBlockedIP(ip); got != tc.want {
				t.Fatalf("IsBlockedIP(%s) = %v, want %v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestResolvePublicRejectsAnyBlockedAddress(t *testing.T) {
	resolver := stubResolver{addrs: []net.IPAddr{
		{IP: net.ParseIP("93.184.216.34")},
		{IP: net.ParseIP("127.0.0.1")},
	}}
	_, err := ResolvePublic(context.Background(), resolver, "example.com")
	if !errors.Is(err, ErrHostBlocked) {
		t.Fatalf("err = %v, want ErrHostBlocked", err)
	}
}

func TestResolvePublicReturnsAddressesWhenAllPublic(t *testing.T) {
	resolver := stubResolver{addrs: []net.IPAddr{
		{IP: net.ParseIP("93.184.216.34")},
		{IP: net.ParseIP("2606:2800:220:1:248:1893:25c8:1946")},
	}}
	got, err := ResolvePublic(context.Background(), resolver, "example.com")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d addresses", len(got))
	}
}

func TestResolvePublicEmptyAddresses(t *testing.T) {
	_, err := ResolvePublic(context.Background(), stubResolver{}, "example.com")
	if !errors.Is(err, ErrNoAddresses) {
		t.Fatalf("err = %v, want ErrNoAddresses", err)
	}
}

func TestResolvePublicContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := stubResolver{err: ctx.Err()}
	_, err := ResolvePublic(ctx, resolver, "example.com")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestResolvePublicDNSLookupError(t *testing.T) {
	resolver := stubResolver{err: errors.New("dns fail")}
	_, err := ResolvePublic(context.Background(), resolver, "example.com")
	if err == nil || !errors.Is(err, resolver.err) {
		t.Fatalf("err = %v", err)
	}
}

func TestDefaultResolverNonNil(t *testing.T) {
	if DefaultResolver() == nil {
		t.Fatal("default resolver is nil")
	}
}
