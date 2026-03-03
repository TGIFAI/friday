package session

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokens(nil); got != 0 {
		t.Errorf("EstimateTokens(nil) = %d, want 0", got)
	}
	if got := EstimateTokens([]*schema.Message{}); got != 0 {
		t.Errorf("EstimateTokens([]) = %d, want 0", got)
	}
}

func TestEstimateTokens_TextOnly(t *testing.T) {
	msgs := []*schema.Message{
		{Role: schema.User, Content: "Hello world"},   // 11 bytes → 11/4 = 2 tokens
		{Role: schema.Assistant, Content: "Hi there!"}, // 9 bytes → 9/4 = 2 tokens
	}
	got := EstimateTokens(msgs)
	// 11/4 + 9/4 = 2 + 2 = 4 (per-message integer division)
	if got != 4 {
		t.Errorf("EstimateTokens = %d, want 4", got)
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	if got := EstimateMessageTokens(nil); got != 0 {
		t.Errorf("EstimateMessageTokens(nil) = %d, want 0", got)
	}
	msg := &schema.Message{Role: schema.User, Content: "Hello world"} // 11 bytes → 2
	if got := EstimateMessageTokens(msg); got != 2 {
		t.Errorf("EstimateMessageTokens = %d, want 2", got)
	}
}

func TestEstimateTokens_WithToolCalls(t *testing.T) {
	msgs := []*schema.Message{
		{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{
				{
					ID: "call_1",
					Function: schema.FunctionCall{
						Name:      "file_read",
						Arguments: `{"path":"/tmp/test.txt"}`,
					},
				},
			},
		},
	}
	got := EstimateTokens(msgs)
	if got <= 0 {
		t.Errorf("EstimateTokens with tool calls should be > 0, got %d", got)
	}
}

func TestEstimateTokens_Chinese(t *testing.T) {
	// Chinese characters are multi-byte but we count len(string) which is bytes.
	// "你好世界" = 12 bytes in UTF-8 → 12/4 = 3 tokens
	msgs := []*schema.Message{
		{Role: schema.User, Content: "你好世界"},
	}
	got := EstimateTokens(msgs)
	if got != 3 {
		t.Errorf("EstimateTokens(Chinese) = %d, want 3", got)
	}
}
