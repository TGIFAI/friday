package channel

// SendOptions holds optional parameters for SendMessage.
type SendOptions struct {
	ReplyToMsgID string
}

// SendOption is a functional option for SendMessage.
type SendOption func(*SendOptions)

// WithReplyTo sets the message ID to reply to.
func WithReplyTo(msgID string) SendOption {
	return func(o *SendOptions) {
		o.ReplyToMsgID = msgID
	}
}

// ApplySendOptions builds a SendOptions from the given options.
func ApplySendOptions(opts []SendOption) SendOptions {
	var o SendOptions
	for _, fn := range opts {
		fn(&o)
	}
	return o
}
