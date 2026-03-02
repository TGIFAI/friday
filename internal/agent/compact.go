package agent

import (
	"context"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

const preFlushMaxIterations = 3

// maybeCompact checks whether the prompt messages exceed the context budget
// and, if so, runs the compaction pipeline: pre-flush → summarize → compact.
// Returns the (possibly rebuilt) prompt messages.
func (ag *Agent) maybeCompact(
	ctx context.Context,
	p provider.Provider,
	modelSpec *provider.ModelSpec,
	sess *session.Session,
	promptMsgs []*schema.Message,
	userMsg *schema.Message,
) []*schema.Message {
	threshold := ag.contextBudget - ag.reserveTokens
	if threshold <= 0 {
		return promptMsgs
	}

	allMsgs := make([]*schema.Message, 0, len(promptMsgs)+1)
	allMsgs = append(allMsgs, promptMsgs...)
	allMsgs = append(allMsgs, userMsg)
	estimated := session.EstimateTokens(allMsgs)
	if estimated <= threshold {
		return promptMsgs
	}

	logs.CtxInfo(ctx, "[agent:%s] compaction triggered: estimated %d tokens > threshold %d",
		ag.id, estimated, threshold)

	// Step 1: Pre-flush — give LLM a chance to persist important info.
	ag.runPreFlush(ctx, p, modelSpec, promptMsgs, userMsg)

	// Step 2: Calculate keepCount.
	history := sess.History()
	keepBudget := threshold / 2
	keepCount := calculateKeepCount(history, keepBudget)

	// Step 3: Generate summary of old messages.
	oldMsgs := history
	if keepCount < len(history) {
		oldMsgs = history[:len(history)-keepCount]
	}

	summary := ag.generateSummary(ctx, p, modelSpec, oldMsgs)
	if summary == nil {
		// Fallback: trim without summary.
		logs.CtxWarn(ctx, "[agent:%s] summary generation failed, falling back to trim", ag.id)
		summary = &schema.Message{
			Role:    schema.Assistant,
			Content: "[Earlier conversation history was trimmed due to context limits]",
		}
	}

	// Step 4: Compact the session.
	sess.Compact(summary, keepCount)
	logs.CtxInfo(ctx, "[agent:%s] compaction complete: kept %d messages, removed %d",
		ag.id, keepCount, len(history)-keepCount)

	// Rebuild prompt messages with compacted history.
	return ag.buildMessages(ctx, sess, nil, p.Type())
}

// runPreFlush runs a short agent loop allowing the LLM to persist important
// information before compaction. Messages from this turn are NOT saved to session.
func (ag *Agent) runPreFlush(
	ctx context.Context,
	p provider.Provider,
	modelSpec *provider.ModelSpec,
	promptMsgs []*schema.Message,
	userMsg *schema.Message,
) {
	flushMsgs := make([]*schema.Message, 0, len(promptMsgs)+2)
	flushMsgs = append(flushMsgs, promptMsgs...)
	flushMsgs = append(flushMsgs, userMsg)
	flushMsgs = append(flushMsgs, &schema.Message{
		Role:    schema.System,
		Content: preFlushPrompt,
	})

	for iter := 0; iter < preFlushMaxIterations; iter++ {
		resp, err := p.Generate(ctx, modelSpec.ModelName, flushMsgs)
		if err != nil {
			logs.CtxWarn(ctx, "[agent:%s] pre-flush LLM call failed: %v", ag.id, err)
			return
		}
		if resp == nil {
			return
		}

		// Check for skip sentinel.
		if strings.Contains(resp.Content, flushSkipSentinel) {
			logs.CtxDebug(ctx, "[agent:%s] pre-flush: LLM signaled FLUSH_SKIP", ag.id)
			return
		}

		// If LLM made tool calls, execute them.
		if len(resp.ToolCalls) > 0 {
			flushMsgs = append(flushMsgs, resp)
			for _, call := range resp.ToolCalls {
				res, callErr := ag.tools.ExecuteToolCall(ctx, &call)
				callMsg := &schema.Message{
					Role:       schema.Tool,
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
				}
				if callErr != nil {
					callMsg.Content = "ERROR: " + callErr.Error()
				} else {
					callMsg.Content = "{}"
					if res != nil {
						if s, ok := res.(string); ok {
							callMsg.Content = s
						}
					}
				}
				flushMsgs = append(flushMsgs, callMsg)
			}
			continue
		}

		// No tool calls, LLM is done.
		return
	}
}

// generateSummary asks the LLM to summarize old messages. Returns nil on failure.
func (ag *Agent) generateSummary(
	ctx context.Context,
	p provider.Provider,
	modelSpec *provider.ModelSpec,
	oldMsgs []*schema.Message,
) *schema.Message {
	summaryMsgs := make([]*schema.Message, 0, len(oldMsgs)+1)
	summaryMsgs = append(summaryMsgs, &schema.Message{
		Role:    schema.System,
		Content: summaryPrompt,
	})
	summaryMsgs = append(summaryMsgs, oldMsgs...)

	resp, err := p.Generate(ctx, modelSpec.ModelName, summaryMsgs)
	if err != nil {
		logs.CtxWarn(ctx, "[agent:%s] summary generation failed: %v", ag.id, err)
		return nil
	}
	if resp == nil || strings.TrimSpace(resp.Content) == "" {
		return nil
	}

	return &schema.Message{
		Role:    schema.Assistant,
		Content: resp.Content,
	}
}

// calculateKeepCount determines how many recent messages to keep based on
// a token budget. Always keeps at least 2 complete turns.
func calculateKeepCount(messages []*schema.Message, tokenBudget int) int {
	if len(messages) == 0 {
		return 0
	}

	used := 0
	count := 0
	minKeep := findMinKeepForTurns(messages, 2)

	for i := len(messages) - 1; i >= 0; i-- {
		msgTokens := session.EstimateTokens([]*schema.Message{messages[i]})
		if used+msgTokens > tokenBudget && count >= minKeep {
			break
		}
		used += msgTokens
		count++
	}

	if count < minKeep {
		count = minKeep
	}
	if count > len(messages) {
		count = len(messages)
	}
	return count
}

// findMinKeepForTurns returns the minimum number of messages from the tail
// needed to include at least n complete user→assistant turns.
func findMinKeepForTurns(messages []*schema.Message, n int) int {
	turns := 0
	count := 0
	for i := len(messages) - 1; i >= 0; i-- {
		count++
		if messages[i].Role == schema.User {
			turns++
			if turns >= n {
				return count
			}
		}
	}
	return count // all messages if fewer than n turns
}
