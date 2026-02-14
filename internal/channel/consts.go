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

type Message struct {
	ID          string
	ChannelID   string // ChannelID
	ChannelType Type
	UserID      string
	ChatID      string
	Content     string
	SessionKey  string
	Metadata    map[string]string
}

type Response struct {
	ID               string
	ChatID           string
	Content          string
	Error            error
	Metadata         map[string]string
	Model            string
	Provider         string
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
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
