package agent

import (
	"context"

	"github.com/bytedance/sonic"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/tgifai/friday/internal/agent/session"
	"github.com/tgifai/friday/internal/channel"
	"github.com/tgifai/friday/internal/pkg/logs"
	"github.com/tgifai/friday/internal/provider"
)

const maxIteration = 15

func (ag *Agent) runLoop(ctx context.Context,
	p provider.Provider,
	ms *provider.ModelSpec,
	sess *session.Session,
	msg *channel.Message,
) (*channel.Response, error) {
	// The current user message has already been appended to the session in ProcessMessage.
	msgs := ag.buildMessages(sess, msg, false)

	logs.CtxDebug(ctx, "[agent:%s] sending to provider %s:%s, messages count: %d",
		ag.id, ms.ProviderID, ms.ModelName, len(msgs))
	/*
		for i, m := range msgs {
			logs.CtxDebug(ctx, "[agent:%s] message[%d] role=%s, content_length=%d, preview: %s",
				ag.id, i, m.Role, len(m.Content), m.Content)
		}
	*/

	var finalResp *schema.Message
	opts := []model.Option{
		model.WithTools(ag.tools.ListToolInfos()),
		model.WithToolChoice(schema.ToolChoiceAllowed),
	}
	for iter := 0; iter < maxIteration; iter++ {
		llmResp, err := p.Generate(ctx, ms.ModelName, msgs, opts...)
		if err != nil || llmResp == nil {
			logs.CtxWarn(ctx, "agent msg generation (nil: %v) failed: %s", llmResp == nil, err)
			return nil, err
		}

		str, _ := sonic.MarshalString(llmResp)
		logs.CtxDebug(ctx, "[agent:%s:%d] llmResp: %+v", ag.id, iter, str)
		if len(llmResp.ToolCalls) > 0 {
			msgs = append(msgs, llmResp)
			for _, call := range llmResp.ToolCalls {
				logs.CtxDebug(ctx, "[agent:%s:%d] call: %+v", ag.id, iter, call)
				res, callErr := ag.tools.ExecuteToolCall(ctx, &call)
				resMsg := &schema.Message{
					Role:       schema.Tool,
					ToolName:   call.Function.Name,
					ToolCallID: call.ID,
				}
				if callErr != nil {
					logs.CtxWarn(ctx, "agent tool call failed: %s", callErr)
					resMsg.Content = "ERROR: " + callErr.Error()
				} else {
					resMsg.Content, _ = sonic.MarshalString(res)
				}
				msgs = append(msgs, resMsg)
			}
			continue
		}

		finalResp = llmResp
		break
	}

	if finalResp == nil {
		return nil, nil
	}

	sess.Append(finalResp)
	return &channel.Response{
		ID:       msg.ID,
		Content:  finalResp.Content,
		Model:    ms.ModelName,
		Provider: ms.ProviderID,
	}, nil
}
