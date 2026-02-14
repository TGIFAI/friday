package channel

import (
	"context"
)

// Channel defines a runtime adapter between Friday and a chat platform.
// Implementations are responsible for receiving inbound events and sending
// outbound responses for a specific channel provider (for example Telegram).
type Channel interface {
	// ID returns the unique configured channel identifier.
	ID() string

	// Type returns the channel provider type used for routing.
	Type() Type

	// Start begins the channel receive loop and should block until the context
	// is canceled or a fatal error occurs.
	Start(ctx context.Context) error

	// Stop gracefully shuts down channel resources.
	Stop(ctx context.Context) error

	// SendMessage sends text content to the target chat.
	// chatID is provider-specific and is passed as a string for portability.
	SendMessage(ctx context.Context, chatID string, content string) error

	// SendChatAction sends a transient user-visible activity state
	// (for example "typing") to the target chat.
	// Implementations that do not support this should return ErrUnsupportedOperation.
	SendChatAction(ctx context.Context, chatID string, action ChatAction) error

	// ReactMessage adds or updates a reaction on a message in the target chat.
	// messageID and reaction format are provider-specific.
	// Implementations that do not support this should return ErrUnsupportedOperation.
	ReactMessage(ctx context.Context, chatID string, messageID string, reaction string) error

	// RegisterMessageHandler registers the inbound message callback.
	// The handler is invoked for each incoming normalized Message.
	RegisterMessageHandler(handler func(ctx context.Context, msg *Message) error) error
}
