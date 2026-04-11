package gowa

import "time"

// ────────────────────────────────────────────────────────────────────────────────
// BaseUserUpdate
// Mirrors pywa's BaseUserUpdate dataclass — the common parent of all user-facing
// update types (Message, CallbackButton, CallbackSelection, ChatOpened, etc.).
// ────────────────────────────────────────────────────────────────────────────────

// BaseUserUpdate contains fields present on every user-originated update.
// Embed this in concrete update types.
//
// Fields:
//   - ID:              The message/update ID (wamid.XXX=).
//   - Metadata:        The receiving phone number metadata.
//   - From:            The sender.
//   - Timestamp:       When the update arrived at WhatsApp servers (UTC).
//   - client:          Back-reference to the owning WhatsApp instance (private).
type BaseUserUpdate struct {
	ID        string
	Metadata  Metadata
	From      User
	Timestamp time.Time

	client *WhatsApp // unexported; set during update construction
}

// MarkAsRead marks the originating message as read.
// Shortcut for WhatsApp.MarkMessageAsRead(u.ID).
//
// Returns:
//   - error if the API call fails.
func (u *BaseUserUpdate) MarkAsRead() error {
	if u.client == nil {
		return ErrNoClient
	}
	return u.client.MarkMessageAsRead(u.ID, "")
}

// IndicateTyping marks the originating message as read and shows a typing indicator.
// Shortcut for WhatsApp.IndicateTyping(u.ID).
//
// Returns:
//   - error if the API call fails.
func (u *BaseUserUpdate) IndicateTyping() error {
	if u.client == nil {
		return ErrNoClient
	}
	return u.client.IndicateTyping(u.ID, "")
}

// React sends an emoji reaction to the originating message.
// Shortcut for WhatsApp.SendReaction.
//
// Parameters:
//   - emoji: the emoji string to react with.
//
// Returns:
//   - *SentReaction containing the reaction message ID.
//   - error if the API call fails.
func (u *BaseUserUpdate) React(emoji string) (*SentReaction, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	return u.client.SendReaction(u.From.WAID, emoji, u.ID, SendReactionOptions{
		Sender: u.Metadata.PhoneNumberID,
	})
}

// Unreact removes the emoji reaction from the originating message.
// Shortcut for WhatsApp.RemoveReaction.
//
// Returns:
//   - *SentReaction confirming the removal.
//   - error if the API call fails.
func (u *BaseUserUpdate) Unreact() (*SentReaction, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	return u.client.RemoveReaction(u.From.WAID, u.ID, RemoveReactionOptions{
		Sender: u.Metadata.PhoneNumberID,
	})
}

// Reply sends a text reply quoting the originating message.
// Shortcut for WhatsApp.SendMessage with ReplyToMessageID set.
//
// Parameters:
//   - text: reply body text.
//   - opts: optional SendMessageOptions.
//
// Returns:
//   - *SentMessage on success.
//   - error if the API call fails.
func (u *BaseUserUpdate) Reply(text string, opts ...SendMessageOptions) (*SentMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := SendMessageOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	o.ReplyToMessageID = u.ID
	o.Sender = u.Metadata.PhoneNumberID
	return u.client.SendMessage(u.From.WAID, text, o)
}

// BlockSender blocks the user who sent this update.
// Shortcut for WhatsApp.BlockUsers.
//
// Returns:
//   - error if the block fails.
func (u *BaseUserUpdate) BlockSender() error {
	if u.client == nil {
		return ErrNoClient
	}
	_, err := u.client.BlockUsers([]string{u.From.WAID})
	return err
}

// ────────────────────────────────────────────────────────────────────────────────
// Message
// Mirrors pywa's Message dataclass — the richest update type.
// ────────────────────────────────────────────────────────────────────────────────

// Message is received when a user sends a message of any content type.
// Maps to pywa's Message dataclass.
//
// Fields:
//   - BaseUserUpdate:      Embedded common fields.
//   - Type:                The message content type.
//   - ReplyToMessage:      Context if this message replies to another (nil otherwise).
//   - Forwarded:           True if the message was forwarded.
//   - ForwardedManyTimes:  True if forwarded more than 5 times.
//   - Text:                Non-nil for text messages.
//   - Image:               Non-nil for image messages.
//   - Video:               Non-nil for video messages.
//   - Sticker:             Non-nil for sticker messages.
//   - Document:            Non-nil for document messages.
//   - Audio:               Non-nil for audio/voice messages.
//   - Reaction:            Non-nil for reaction messages.
//   - Location:            Non-nil for location messages.
//   - Contacts:            Non-nil for contact card messages.
//   - Order:               Non-nil for product order messages.
//   - Referral:            Non-nil when the message originates from an ad click.
//   - Unsupported:         Non-nil for unsupported message types.
//   - Error:               Non-nil when the message contains an embedded error.
type Message struct {
	BaseUserUpdate

	Type               MessageType
	ReplyToMessage     *ReplyToMessage
	Forwarded          bool
	ForwardedManyTimes bool

	Text        *string
	Image       *Image
	Video       *Video
	Sticker     *Sticker
	Document    *Document
	Audio       *Audio
	Reaction    *Reaction
	Location    *Location
	Contacts    []Contact
	Order       *Order
	Referral    *Referral
	Unsupported *Unsupported
	Error       *WhatsAppError
}

// Voice is shorthand for the Audio field when it contains a voice note.
//
// Returns:
//   - *Audio if the audio field is a voice note, nil otherwise.
func (m *Message) Voice() *Audio {
	if m.Audio != nil && m.Audio.Voice {
		return m.Audio
	}
	return nil
}

// HasMedia returns true when the message contains any media attachment.
//
// Returns:
//   - true if Image, Video, Sticker, Document, or Audio is non-nil.
func (m *Message) HasMedia() bool {
	return m.Image != nil || m.Video != nil || m.Sticker != nil ||
		m.Document != nil || m.Audio != nil
}

// IsReply returns true if this message is a reply or reaction to another message.
//
// Returns:
//   - true if ReplyToMessage is set or this is a reaction.
func (m *Message) IsReply() bool {
	return m.ReplyToMessage != nil || m.Reaction != nil
}

// Caption returns the caption from an image, video, or document message.
//
// Returns:
//   - caption string or "" if none.
func (m *Message) Caption() string {
	if m.Image != nil {
		return m.Image.Caption
	}
	if m.Video != nil {
		return m.Video.Caption
	}
	if m.Document != nil {
		return m.Document.Caption
	}
	return ""
}
