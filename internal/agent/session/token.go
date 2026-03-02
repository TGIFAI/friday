package session

import "github.com/cloudwego/eino/schema"

// EstimateTokens returns a rough token count for the given messages.
// Uses byte-length / 4 as a heuristic (English ~1:4, Chinese ~1:2).
// Precision is not required — this is used for threshold detection only.
func EstimateTokens(msgs []*schema.Message) int {
	if len(msgs) == 0 {
		return 0
	}
	total := 0
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		total += len(msg.Content)
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Name)
			total += len(tc.Function.Arguments)
		}
	}
	return total / 4
}
