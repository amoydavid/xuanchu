package main

import "testing"

func TestResolveVersionWithLdflags(t *testing.T) {
	orig := version
	version = "1.2.3"
	defer func() { version = orig }()
	if got := resolveVersion(); got != "1.2.3" {
		t.Errorf("got %q, want %q", got, "1.2.3")
	}
}

func TestResolveVersionFallback(t *testing.T) {
	orig := version
	version = ""
	defer func() { version = orig }()
	got := resolveVersion()
	if got == "" {
		t.Error("resolveVersion returned empty string")
	}
}
