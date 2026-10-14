package telegram

// User is a Telegram user or bot.
type User struct {
	ID           int64  `json:"id"`
	IsBot        bool   `json:"is_bot"`
	FirstName    string `json:"first_name"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

// Chat is a conversation.
type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

// PhotoSize is one resolution of a photo.
type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size,omitempty"`
}

// Document is a file sent as a document (e.g. an uncompressed screenshot).
type Document struct {
	FileID   string `json:"file_id"`
	MimeType string `json:"mime_type,omitempty"`
	FileSize int64  `json:"file_size,omitempty"`
}

// MessageOrigin describes where a forwarded message came from.
type MessageOrigin struct {
	Type string `json:"type"` // user, hidden_user, chat, channel
}

// Message is a Telegram message.
type Message struct {
	MessageID     int64          `json:"message_id"`
	From          *User          `json:"from,omitempty"`
	Chat          Chat           `json:"chat"`
	Date          int64          `json:"date"`
	Text          string         `json:"text,omitempty"`
	Caption       string         `json:"caption,omitempty"`
	Photo         []PhotoSize    `json:"photo,omitempty"`
	Document      *Document      `json:"document,omitempty"`
	ForwardOrigin *MessageOrigin `json:"forward_origin,omitempty"`
}

// CallbackQuery is an inline-button press.
type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message,omitempty"`
	Data    string   `json:"data,omitempty"`
}

// Update is one incoming event.
type Update struct {
	UpdateID      int64          `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

// InlineKeyboardButton is a button under a message.
type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data,omitempty"`
	URL          string `json:"url,omitempty"`
}

// InlineKeyboardMarkup is a grid of inline buttons.
type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// ReplyParameters points a reply at an earlier message.
type ReplyParameters struct {
	MessageID                int64 `json:"message_id"`
	AllowSendingWithoutReply bool  `json:"allow_sending_without_reply,omitempty"`
}

// LinkPreviewOptions controls link previews.
type LinkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled,omitempty"`
}

// SendMessage are the parameters of sendMessage.
type SendMessage struct {
	ChatID             int64                 `json:"chat_id"`
	Text               string                `json:"text"`
	ParseMode          string                `json:"parse_mode,omitempty"`
	ReplyMarkup        *InlineKeyboardMarkup `json:"reply_markup,omitempty"`
	ReplyParameters    *ReplyParameters      `json:"reply_parameters,omitempty"`
	LinkPreviewOptions *LinkPreviewOptions   `json:"link_preview_options,omitempty"`
}
