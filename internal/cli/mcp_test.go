package cli

import "testing"

func TestMCPStdioCommandRegistersShutdownFlags(t *testing.T) {
	cmd := newMCPStdioCommand(Options{})
	for _, name := range []string{"shutdown-timeout", "shutdown-force-timeout"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Fatalf("mcp stdio flag %q not registered", name)
		}
	}
}
