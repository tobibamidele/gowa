package gowa

import "time"

// ────────────────────────────────────────────────────────────────────────────────
// Reply shortcut methods on BaseUserUpdate
// Mirrors every reply_* helper defined in pywa/types/base_update.py.
// All methods quote the originating message (ReplyToMessageID = u.ID).
// ────────────────────────────────────────────────────────────────────────────────

// ReplyImage sends an image that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_image.
//
// Parameters:
//   - image:   URL string, file path string, or []byte.
//   - caption: optional caption.
//   - opts:    optional SendMediaOptions (Sender is auto-set to the receiving phone ID).
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (u *BaseUserUpdate) ReplyImage(image any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := mergeMediaOpts(u, opts)
	return u.client.SendImage(u.From.WAID, image, caption, o)
}

// ReplyVideo sends a video that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_video.
//
// Parameters:
//   - video:   URL string, file path string, or []byte.
//   - caption: optional caption.
//   - opts:    optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (u *BaseUserUpdate) ReplyVideo(video any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := mergeMediaOpts(u, opts)
	return u.client.SendVideo(u.From.WAID, video, caption, o)
}

// ReplyDocument sends a document that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_document.
//
// Parameters:
//   - document: URL string, file path string, or []byte.
//   - caption:  optional caption.
//   - opts:     optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (u *BaseUserUpdate) ReplyDocument(document any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := mergeMediaOpts(u, opts)
	return u.client.SendDocument(u.From.WAID, document, caption, o)
}

// ReplyAudio sends an audio message that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_audio.
//
// Parameters:
//   - audio: URL string, file path string, or []byte.
//   - opts:  optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (u *BaseUserUpdate) ReplyAudio(audio any, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := mergeMediaOpts(u, opts)
	return u.client.SendAudio(u.From.WAID, audio, o)
}

// ReplyVoice sends a voice note that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_voice.
//
// Parameters:
//   - voice: URL string, file path string, or []byte (OGG/OPUS).
//   - opts:  optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (u *BaseUserUpdate) ReplyVoice(voice any, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := mergeMediaOpts(u, opts)
	return u.client.SendVoice(u.From.WAID, voice, o)
}

// ReplySticker sends a sticker that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_sticker.
//
// Parameters:
//   - sticker: URL string, file path string, or []byte (WebP).
//   - opts:    optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (u *BaseUserUpdate) ReplySticker(sticker any, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := mergeMediaOpts(u, opts)
	return u.client.SendSticker(u.From.WAID, sticker, o)
}

// ReplyLocation sends a location message that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_location.
//
// Parameters:
//   - latitude:  decimal latitude.
//   - longitude: decimal longitude.
//   - name:      optional location name.
//   - address:   optional address text.
//
// Returns:
//   - *SentMessage.
//   - error.
func (u *BaseUserUpdate) ReplyLocation(latitude, longitude float64, name, address string) (*SentMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	return u.client.SendLocation(u.From.WAID, latitude, longitude, name, address,
		SendLocationOptions{
			ReplyToMessageID: u.ID,
			Sender:           u.Metadata.PhoneNumberID,
		})
}

// ReplyLocationRequest sends a location-request message that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_location_request.
//
// Parameters:
//   - text: body text shown with the "Send Location" button.
//
// Returns:
//   - *SentLocationRequest.
//   - error.
func (u *BaseUserUpdate) ReplyLocationRequest(text string) (*SentLocationRequest, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	return u.client.RequestLocation(u.From.WAID, text, SendLocationOptions{
		ReplyToMessageID: u.ID,
		Sender:           u.Metadata.PhoneNumberID,
	})
}

// ReplyContact sends one or more contact cards that quote the originating message.
// Mirrors pywa's BaseUserUpdate.reply_contact.
//
// Parameters:
//   - contacts: one or more Contact values.
//
// Returns:
//   - *SentMessage.
//   - error.
func (u *BaseUserUpdate) ReplyContact(contacts []Contact) (*SentMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	return u.client.SendContact(u.From.WAID, contacts, SendContactOptions{
		ReplyToMessageID: u.ID,
		Sender:           u.Metadata.PhoneNumberID,
	})
}

// ReplyCatalog sends a catalog message that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_catalog.
//
// Parameters:
//   - body:   body text.
//   - footer: optional footer.
//   - opts:   optional SendCatalogOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (u *BaseUserUpdate) ReplyCatalog(body, footer string, opts ...SendCatalogOptions) (*SentMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := SendCatalogOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	o.ReplyToMessageID = u.ID
	o.Sender = u.Metadata.PhoneNumberID
	return u.client.SendCatalog(u.From.WAID, body, footer, o)
}

// ReplyProduct sends a single product card that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_product.
//
// Parameters:
//   - catalogID: the catalog ID.
//   - sku:       product retailer SKU.
//   - opts:      optional SendProductOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (u *BaseUserUpdate) ReplyProduct(catalogID, sku string, opts ...SendProductOptions) (*SentMessage, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := SendProductOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	o.ReplyToMessageID = u.ID
	o.Sender = u.Metadata.PhoneNumberID
	return u.client.SendProduct(u.From.WAID, catalogID, sku, o)
}

// ReplyTemplate sends a template message that quotes the originating message.
// Mirrors pywa's BaseUserUpdate.reply_template.
//
// Parameters:
//   - name:     template name.
//   - language: BCP-47 language code (e.g. "en_US").
//   - opts:     optional SendTemplateOptions.
//
// Returns:
//   - *SentTemplate.
//   - error.
func (u *BaseUserUpdate) ReplyTemplate(name, language string, opts ...SendTemplateOptions) (*SentTemplate, error) {
	if u.client == nil {
		return nil, ErrNoClient
	}
	o := SendTemplateOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	o.ReplyToMessageID = u.ID
	o.Sender = u.Metadata.PhoneNumberID
	return u.client.SendTemplate(u.From.WAID, name, language, o)
}

// WaitForReply blocks until the sender sends a reply to this message,
// or until the timeout expires.
// Mirrors pywa's SentMessage.wait_for_reply.
//
// Parameters:
//   - wa:      the active WhatsApp client.
//   - filters: optional Filter[*Message] to apply (nil = accept any message).
//   - timeout: optional wait duration; omit or pass 0 for no timeout.
//
// Returns:
//   - *Message: the reply.
//   - error: *ListenerTimeout, *ListenerCanceled, or *ListenerStopped.
func (u *BaseUserUpdate) WaitForReply(wa *WhatsApp, filters Filter[*Message], timeout ...time.Duration) (*Message, error) {
	var dur time.Duration
	if len(timeout) > 0 {
		dur = timeout[0]
	}
	return wa.Listen(ListenOptions{
		SenderWAID:  u.From.WAID,
		RecipientID: u.Metadata.PhoneNumberID,
		Filters:     filters,
		Timeout:     dur,
	})
}

// ── helper ────────────────────────────────────────────────────────────────────

// mergeMediaOpts injects the reply-to ID and sender from the update into opts.
func mergeMediaOpts(u *BaseUserUpdate, opts []SendMediaOptions) SendMediaOptions {
	o := SendMediaOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.ReplyToMessageID == "" {
		o.ReplyToMessageID = u.ID
	}
	if o.Sender == "" {
		o.Sender = u.Metadata.PhoneNumberID
	}
	return o
}
