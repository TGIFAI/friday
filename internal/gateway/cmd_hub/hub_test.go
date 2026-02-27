package cmd_hub

import (
	"testing"
)

func TestHub_NewCommand(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	cmd, _, ok := h.Match("/new")
	if !ok {
		t.Fatal("/new command should be registered")
	}
	if cmd.Name != "/new" {
		t.Errorf("command name = %q, want /new", cmd.Name)
	}
}

func TestHub_NewCommandCaseInsensitive(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	_, _, ok := h.Match("/NEW")
	if !ok {
		t.Fatal("/NEW (uppercase) should match /new")
	}
}

func TestHub_NewCommandWithBotSuffix(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	_, _, ok := h.Match("/new@mybot")
	if !ok {
		t.Fatal("/new@mybot should match /new")
	}
}

func TestHub_AllBuiltinCommands(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	expected := []string{"/start", "/help", "/status", "/cronjob", "/new"}
	for _, name := range expected {
		if _, _, ok := h.Match(name); !ok {
			t.Errorf("expected builtin command %s to be registered", name)
		}
	}
}

func TestHub_Unregister(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	h.Unregister("/new")
	if _, _, ok := h.Match("/new"); ok {
		t.Fatal("/new should not match after unregister")
	}
}

func TestHub_NoMatchEmpty(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	if _, _, ok := h.Match(""); ok {
		t.Fatal("empty string should not match")
	}
}

func TestHub_NoMatchPlainText(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	if _, _, ok := h.Match("hello world"); ok {
		t.Fatal("plain text should not match")
	}
}

func TestHub_MatchArgs(t *testing.T) {
	h := NewHub()
	RegisterBuiltins(h)

	_, args, ok := h.Match("/start some arguments")
	if !ok {
		t.Fatal("/start with args should match")
	}
	if args != "some arguments" {
		t.Errorf("args = %q, want %q", args, "some arguments")
	}
}
