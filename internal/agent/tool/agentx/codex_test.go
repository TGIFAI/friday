package agentx

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestCodexBackendName(t *testing.T) {
	b := &CodexBackend{}
	if got := b.Name(); got != "codex" {
		t.Fatalf("Name() = %q, want %q", got, "codex")
	}
}

func TestCodexBackendAvailable(t *testing.T) {
	b := &CodexBackend{}
	_, lookErr := exec.LookPath("codex")
	want := lookErr == nil
	if got := b.Available(); got != want {
		t.Fatalf("Available() = %v, want %v (LookPath error: %v)", got, want, lookErr)
	}
}

func TestCodexBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		req  *RunRequest
		want []string
	}{
		{
			name: "basic",
			req:  &RunRequest{Prompt: "hello"},
			want: []string{"exec", "hello", "--json", "--dangerously-bypass-approvals-and-sandbox"},
		},
		{
			name: "with resume",
			req:  &RunRequest{Prompt: "hello", ResumeID: "sess-1"},
			want: []string{"exec", "resume", "sess-1", "hello", "--json", "--dangerously-bypass-approvals-and-sandbox"},
		},
	}

	b := &CodexBackend{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildArgs(tt.req)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCodexParseJSONL(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		exitCode int
		want     *RunResult
	}{
		{
			name: "extracts last assistant message and thread_id",
			raw: `{"type":"thread.started","thread_id":"t-1"}
{"type":"item.created","item":{"type":"message","role":"assistant","content":[{"type":"text","text":"working..."}]}}
{"type":"item.created","item":{"type":"message","role":"assistant","content":[{"type":"text","text":"all done!"}]}}
{"type":"turn.completed"}`,
			exitCode: 0,
			want: &RunResult{
				CLISessionID: "t-1",
				Output:       "all done!",
				ExitCode:     0,
			},
		},
		{
			name: "extracts thread_id as CLISessionID",
			raw: `{"type":"thread.started","thread_id":"sess-abc"}
{"type":"item.created","item":{"type":"message","role":"assistant","content":[{"type":"text","text":"hello"}]}}`,
			exitCode: 0,
			want: &RunResult{
				CLISessionID: "sess-abc",
				Output:       "hello",
				ExitCode:     0,
			},
		},
		{
			name:     "falls back to raw text when no assistant messages",
			raw:      `{"type":"thread.started","thread_id":"t-2"}`,
			exitCode: 1,
			want: &RunResult{
				CLISessionID: "t-2",
				Output:       `{"type":"thread.started","thread_id":"t-2"}`,
				ExitCode:     1,
			},
		},
		{
			name:     "falls back to raw text on invalid json",
			raw:      "not json at all",
			exitCode: 0,
			want: &RunResult{
				Output:   "not json at all",
				ExitCode: 0,
			},
		},
	}

	b := &CodexBackend{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.parseJSONL(tt.raw, tt.exitCode)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseJSONL() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
