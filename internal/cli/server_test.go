package cli

import (
	"strings"
	"testing"
	"time"

	"git.dajee.net/dajee/xuanchu/internal/config"
)

func TestServerDispatcherRuntimeOptionsUseConfigAndFlags(t *testing.T) {
	cfg := config.Config{
		NotificationDispatcher: config.DispatcherConfig{
			MaxConcurrency: 1,
			BatchSize:      50,
			PrefetchFactor: 1,
			PollInterval:   5 * time.Second,
			ClaimTTL:       5 * time.Minute,
		},
		HookDispatcher: config.DispatcherConfig{
			MaxConcurrency: 2,
			BatchSize:      40,
			PrefetchFactor: 1,
			PollInterval:   6 * time.Second,
			ClaimTTL:       6 * time.Minute,
		},
	}
	flags := serverDispatcherFlagOverrides{
		NotificationMaxConcurrency: ptrInt(4),
		NotificationBatchSize:      ptrInt(30),
		NotificationPrefetchFactor: ptrInt(3),
		NotificationClaimTTL:       ptrDuration(9 * time.Minute),
		LegacyNotificationInterval: ptrDuration(7 * time.Second),
	}

	opts, err := buildServerDispatcherRuntimeOptions(cfg, flags)
	if err != nil {
		t.Fatalf("buildServerDispatcherRuntimeOptions() error = %v", err)
	}
	if opts.Notification.MaxConcurrency != 4 {
		t.Fatalf("notification max = %d, want 4", opts.Notification.MaxConcurrency)
	}
	if opts.Notification.BatchSize != 30 {
		t.Fatalf("notification batch = %d, want 30", opts.Notification.BatchSize)
	}
	if opts.Notification.PrefetchFactor != 3 {
		t.Fatalf("notification prefetch = %d, want 3", opts.Notification.PrefetchFactor)
	}
	if opts.Notification.PollInterval != 7*time.Second {
		t.Fatalf("notification interval = %v, want 7s", opts.Notification.PollInterval)
	}
	if opts.Notification.ClaimTTL != 9*time.Minute {
		t.Fatalf("notification claim ttl = %v, want 9m", opts.Notification.ClaimTTL)
	}
	if opts.Hook.MaxConcurrency != 2 {
		t.Fatalf("hook max = %d, want 2", opts.Hook.MaxConcurrency)
	}
	if opts.Hook.BatchSize != 40 {
		t.Fatalf("hook batch = %d, want config 40", opts.Hook.BatchSize)
	}
	if opts.Hook.PollInterval != 7*time.Second {
		t.Fatalf("hook interval = %v, want legacy 7s when hook flag absent", opts.Hook.PollInterval)
	}
	if opts.SinkLimiter == nil || opts.Notification.SinkLimiter != opts.Hook.SinkLimiter {
		t.Fatal("notification and hook dispatchers must share one sink limiter")
	}
	if opts.Notification.DefaultSinkConcurrency != opts.Hook.DefaultSinkConcurrency {
		t.Fatalf("default sink concurrency mismatch: notification=%d hook=%d", opts.Notification.DefaultSinkConcurrency, opts.Hook.DefaultSinkConcurrency)
	}
	if opts.Notification.DefaultSinkConcurrency != 2 {
		t.Fatalf("default sink concurrency = %d, want min(notification, hook)=2", opts.Notification.DefaultSinkConcurrency)
	}
}

func TestServerHookDispatcherIntervalFlagWinsOverLegacyInterval(t *testing.T) {
	cfg := config.Config{
		NotificationDispatcher: config.DispatcherConfig{
			MaxConcurrency: 1,
			BatchSize:      50,
			PrefetchFactor: 1,
			PollInterval:   5 * time.Second,
			ClaimTTL:       5 * time.Minute,
		},
		HookDispatcher: config.DispatcherConfig{
			MaxConcurrency: 1,
			BatchSize:      50,
			PrefetchFactor: 1,
			PollInterval:   6 * time.Second,
			ClaimTTL:       5 * time.Minute,
		},
	}
	flags := serverDispatcherFlagOverrides{
		LegacyNotificationInterval: ptrDuration(7 * time.Second),
		HookInterval:               ptrDuration(8 * time.Second),
		HookBatchSize:              ptrInt(25),
		HookPrefetchFactor:         ptrInt(2),
		HookClaimTTL:               ptrDuration(10 * time.Minute),
	}

	opts, err := buildServerDispatcherRuntimeOptions(cfg, flags)
	if err != nil {
		t.Fatalf("buildServerDispatcherRuntimeOptions() error = %v", err)
	}
	if opts.Hook.PollInterval != 8*time.Second {
		t.Fatalf("hook interval = %v, want hook flag 8s", opts.Hook.PollInterval)
	}
	if opts.Hook.BatchSize != 25 {
		t.Fatalf("hook batch = %d, want 25", opts.Hook.BatchSize)
	}
	if opts.Hook.PrefetchFactor != 2 {
		t.Fatalf("hook prefetch = %d, want 2", opts.Hook.PrefetchFactor)
	}
	if opts.Hook.ClaimTTL != 10*time.Minute {
		t.Fatalf("hook claim ttl = %v, want 10m", opts.Hook.ClaimTTL)
	}
}

func TestServerCommandRegistersDispatcherRuntimeFlags(t *testing.T) {
	cmd := newServerCommand(Options{})
	for _, name := range []string{
		"shutdown-timeout",
		"shutdown-force-timeout",
		"notification-dispatcher-max-concurrency",
		"notification-dispatcher-batch-size",
		"notification-dispatcher-prefetch-factor",
		"notification-dispatcher-interval",
		"notification-dispatcher-claim-ttl",
		"hook-dispatcher-max-concurrency",
		"hook-dispatcher-batch-size",
		"hook-dispatcher-prefetch-factor",
		"hook-dispatcher-interval",
		"hook-dispatcher-claim-ttl",
		"mcp-trusted-proxy-host",
	} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("server flag %q not registered", name)
		}
	}
}

func TestServerShutdownOptionsUseConfigAndFlags(t *testing.T) {
	cfg := config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:      30 * time.Second,
			ForceTimeout: 5 * time.Second,
		},
	}
	flags := serverShutdownFlagOverrides{
		Timeout:      ptrDuration(45 * time.Second),
		ForceTimeout: ptrDuration(9 * time.Second),
	}
	opts, err := buildServerShutdownOptions(cfg, flags)
	if err != nil {
		t.Fatalf("buildServerShutdownOptions() error = %v", err)
	}
	if opts.Timeout != 45*time.Second {
		t.Fatalf("Timeout = %v, want 45s", opts.Timeout)
	}
	if opts.ForceTimeout != 9*time.Second {
		t.Fatalf("ForceTimeout = %v, want 9s", opts.ForceTimeout)
	}
}

func TestServerShutdownOptionsRejectInvalidFlags(t *testing.T) {
	cfg := config.Config{
		Shutdown: config.ShutdownConfig{
			Timeout:      30 * time.Second,
			ForceTimeout: 5 * time.Second,
		},
	}
	tests := []struct {
		name  string
		flags serverShutdownFlagOverrides
		want  string
	}{
		{name: "timeout", flags: serverShutdownFlagOverrides{Timeout: ptrDuration(0)}, want: "shutdown-timeout"},
		{name: "force timeout", flags: serverShutdownFlagOverrides{ForceTimeout: ptrDuration(-time.Second)}, want: "shutdown-force-timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildServerShutdownOptions(cfg, tt.flags)
			if err == nil {
				t.Fatal("buildServerShutdownOptions() error = nil, want invalid flag error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestServerDispatcherRuntimeOptionsRejectInvalidFlags(t *testing.T) {
	cfg := config.Config{
		NotificationDispatcher: config.DispatcherConfig{
			MaxConcurrency: 1,
			BatchSize:      50,
			PrefetchFactor: 1,
			PollInterval:   5 * time.Second,
			ClaimTTL:       5 * time.Minute,
		},
		HookDispatcher: config.DispatcherConfig{
			MaxConcurrency: 1,
			BatchSize:      50,
			PrefetchFactor: 1,
			PollInterval:   5 * time.Second,
			ClaimTTL:       5 * time.Minute,
		},
	}
	tests := []struct {
		name  string
		flags serverDispatcherFlagOverrides
		want  string
	}{
		{name: "notification max", flags: serverDispatcherFlagOverrides{NotificationMaxConcurrency: ptrInt(0)}, want: "notification-dispatcher-max-concurrency"},
		{name: "hook batch", flags: serverDispatcherFlagOverrides{HookBatchSize: ptrInt(-1)}, want: "hook-dispatcher-batch-size"},
		{name: "notification prefetch", flags: serverDispatcherFlagOverrides{NotificationPrefetchFactor: ptrInt(0)}, want: "notification-dispatcher-prefetch-factor"},
		{name: "hook interval", flags: serverDispatcherFlagOverrides{HookInterval: ptrDuration(-time.Second)}, want: "hook-dispatcher-interval"},
		{name: "notification ttl", flags: serverDispatcherFlagOverrides{NotificationClaimTTL: ptrDuration(0)}, want: "notification-dispatcher-claim-ttl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildServerDispatcherRuntimeOptions(cfg, tt.flags)
			if err == nil {
				t.Fatal("buildServerDispatcherRuntimeOptions() error = nil, want invalid flag error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tt.want)
			}
		})
	}
}

func TestServerMCPOptionsUseConfigAndFlags(t *testing.T) {
	cfg := config.Config{
		ServerMCP: config.MCPConfig{
			TrustedProxyHosts: []string{"config.example.com"},
		},
	}
	opts, err := buildServerMCPOptions(cfg, nil)
	if err != nil {
		t.Fatalf("buildServerMCPOptions() error = %v", err)
	}
	if strings.Join(opts.TrustedProxyHosts, ",") != "config.example.com" {
		t.Fatalf("TrustedProxyHosts = %#v, want config host", opts.TrustedProxyHosts)
	}

	opts, err = buildServerMCPOptions(cfg, []string{"flag.example.com"})
	if err != nil {
		t.Fatalf("buildServerMCPOptions() flag error = %v", err)
	}
	if strings.Join(opts.TrustedProxyHosts, ",") != "flag.example.com" {
		t.Fatalf("TrustedProxyHosts = %#v, want flag host", opts.TrustedProxyHosts)
	}
}

func TestServerMCPOptionsRejectWildcardFlag(t *testing.T) {
	_, err := buildServerMCPOptions(config.Config{}, []string{"*.example.com"})
	if err == nil {
		t.Fatal("buildServerMCPOptions() error = nil, want wildcard rejection")
	}
	if !strings.Contains(err.Error(), "mcp-trusted-proxy-host") {
		t.Fatalf("error = %q, want mcp-trusted-proxy-host", err.Error())
	}
}

func ptrInt(v int) *int {
	return &v
}

func ptrDuration(v time.Duration) *time.Duration {
	return &v
}
