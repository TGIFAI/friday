package consts

// CtxKey is the type used for context value keys across the framework.
type CtxKey string

const (
	CtxKeyLogID     CtxKey = "log_id"
	CtxKeyAgentID   CtxKey = "agent_id"
	CtxKeyChannelID CtxKey = "channel_id"
	CtxKeyChatID    CtxKey = "chat_id"
)
