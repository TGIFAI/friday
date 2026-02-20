package agentx

import (
	"os/exec"
	"reflect"
	"testing"
)

func TestClaudeCodeBackendName(t *testing.T) {
	b := &ClaudeCodeBackend{}
	if got := b.Name(); got != "claude-code" {
		t.Fatalf("Name() = %q, want %q", got, "claude-code")
	}
}

func TestClaudeCodeBackendAvailable(t *testing.T) {
	b := &ClaudeCodeBackend{}
	_, lookErr := exec.LookPath("claude")
	want := lookErr == nil
	if got := b.Available(); got != want {
		t.Fatalf("Available() = %v, want %v (LookPath error: %v)", got, want, lookErr)
	}
}

func TestClaudeCodeBuildArgs(t *testing.T) {
	tests := []struct {
		name string
		req  *RunRequest
		want []string
	}{
		{
			name: "basic",
			req:  &RunRequest{Prompt: "hello"},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json"},
		},
		{
			name: "with resume",
			req:  &RunRequest{Prompt: "hello", ResumeID: "sess-1"},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json", "--resume", "sess-1"},
		},
		{
			name: "with system prompt",
			req:  &RunRequest{Prompt: "hello", SystemPrompt: "be safe"},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json", "--append-system-prompt", "be safe"},
		},
		{
			name: "with max turns",
			req:  &RunRequest{Prompt: "hello", MaxTurns: 10},
			want: []string{"-p", "hello", "--dangerously-skip-permissions", "--output-format", "json", "--max-turns", "10"},
		},
	}

	b := &ClaudeCodeBackend{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := b.buildArgs(tt.req)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaudeCodeParseResult(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		exitCode int
		want     *RunResult
	}{
		{
			name:     "valid json",
			raw:      `{"result":"all fixed","session_id":"abc-123"}`,
			exitCode: 0,
			want: &RunResult{
				CLISessionID: "abc-123",
				Output:       "all fixed",
				ExitCode:     0,
			},
		},
		{
			name:     "invalid json",
			raw:      "some raw output text",
			exitCode: 0,
			want: &RunResult{
				Output:   "some raw output text",
				ExitCode: 0,
			},
		},
		{
			name:     "non-zero exit",
			raw:      `{"result":"partial","session_id":"def-456"}`,
			exitCode: 1,
			want: &RunResult{
				CLISessionID: "def-456",
				Output:       "partial",
				ExitCode:     1,
			},
		},
	}

	b := &ClaudeCodeBackend{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := b.parseResult(tt.raw, tt.exitCode)
			if err != nil {
				t.Fatalf("parseResult() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseResult() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
