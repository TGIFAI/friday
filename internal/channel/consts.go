package channel

import (
	"errors"
)

var ErrUnsupportedOperation = errors.New("channel operation is not supported")

type Type string

const (
	Telegram Type = "telegram"

	Lark Type = "lark"

	HTTP Type = "http"
)

var SupportedChannels = []Type{
	Telegram,
	Lark,
	HTTP,
}

// AttachmentType identifies the kind of media attached to a message.
type AttachmentType string

const (
	AttachmentImage AttachmentType = "image"
	AttachmentVoice AttachmentType = "voice"
)

// Attachment holds downloaded media content from a channel message.
// Data contains the raw bytes; the agent layer base64-encodes them before
// passing to the LLM. Attachments are NOT persisted to session history.
type Attachment struct {
	Type     AttachmentType
	Data     []byte
	MIMEType string // e.g. "image/jpeg", "audio/ogg"
	FileName string
}

type Message struct {
	ID          string
	ChannelID   string // ChannelID
	ChannelType Type
	UserID      string
	ChatID      string
	Content     string
	SessionKey  string
	Metadata    map[string]string
	Attachments []Attachment
}

type Response struct {
	ID       string
	ChatID   string
	Content  string
	Error    error
	Metadata map[string]string
	Model    string
	Provider string
}

type ChatAction string

const (
	ChatActionTyping          ChatAction = "typing"
	ChatActionUploadPhoto     ChatAction = "upload_photo"
	ChatActionRecordVideo     ChatAction = "record_video"
	ChatActionUploadVideo     ChatAction = "upload_video"
	ChatActionRecordVoice     ChatAction = "record_voice"
	ChatActionUploadVoice     ChatAction = "upload_voice"
	ChatActionUploadDocument  ChatAction = "upload_document"
	ChatActionChooseSticker   ChatAction = "choose_sticker"
	ChatActionFindLocation    ChatAction = "find_location"
	ChatActionRecordVideoNote ChatAction = "record_video_note"
	ChatActionUploadVideoNote ChatAction = "upload_video_note"
)
