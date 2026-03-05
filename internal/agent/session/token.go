package session

import "github.com/cloudwego/eino/schema"

// EstimateMessageTokens returns a rough token count for a single message.
// Uses byte-length / 4 as a heuristic (English ~1:4, Chinese ~1:2).
// Precision is not required — this is used for threshold detection only.
func EstimateMessageTokens(msg *schema.Message) int {
	if msg == nil {
		return 0
	}
	total := len(msg.Content)
	for _, tc := range msg.ToolCalls {
		total += len(tc.Function.Name)
		total += len(tc.Function.Arguments)
	}
	return total / 4
}

// EstimateTokens returns a rough token count for the given messages.
func EstimateTokens(msgs []*schema.Message) int {
	total := 0
	for _, msg := range msgs {
		total += EstimateMessageTokens(msg)
	}
	return total
}
