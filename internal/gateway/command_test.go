package gateway

import (
	"testing"
)

func TestCommandRouter_NewCommand(t *testing.T) {
	r := newCommandRouter()
	registerBuiltinCommands(r)

	cmd, _, ok := r.Match("/new")
	if !ok {
		t.Fatal("/new command should be registered")
	}
	if cmd.Name != "/new" {
		t.Errorf("command name = %q, want /new", cmd.Name)
	}
}

func TestCommandRouter_NewCommandCaseInsensitive(t *testing.T) {
	r := newCommandRouter()
	registerBuiltinCommands(r)

	_, _, ok := r.Match("/NEW")
	if !ok {
		t.Fatal("/NEW (uppercase) should match /new")
	}
}

func TestCommandRouter_NewCommandWithBotSuffix(t *testing.T) {
	r := newCommandRouter()
	registerBuiltinCommands(r)

	_, _, ok := r.Match("/new@mybot")
	if !ok {
		t.Fatal("/new@mybot should match /new")
	}
}

func TestCommandRouter_AllBuiltinCommands(t *testing.T) {
	r := newCommandRouter()
	registerBuiltinCommands(r)

	expected := []string{"/start", "/help", "/status", "/cronjob", "/new"}
	for _, name := range expected {
		if _, _, ok := r.Match(name); !ok {
			t.Errorf("expected builtin command %s to be registered", name)
		}
	}
}
