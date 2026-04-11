package gowa

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────────
// Sentinel errors
// ────────────────────────────────────────────────────────────────────────────────

// ErrNoClient is returned by update shortcut methods when a client is not set.
var ErrNoClient = fmt.Errorf("gowa: update is not associated with a WhatsApp client")

// ErrNoWebhook is returned when webhook operations are attempted without configuration.
var ErrNoWebhook = fmt.Errorf("gowa: client is not configured to receive webhook updates")

// ────────────────────────────────────────────────────────────────────────────────
// Config
// ────────────────────────────────────────────────────────────────────────────────

// Config holds all settings passed to New().
// Mirrors the parameters of pywa's WhatsApp.__init__.
//
// Fields:
//   - Token:               Bearer access token (required unless you only use webhooks).
//   - PhoneID:             Sender phone number ID (required for sending).
//   - BusinessAccountID:   WABA ID (required for template/flow management).
//   - AppID:               Meta app ID (required for callback URL registration).
//   - AppSecret:           Meta app secret (required for signature validation).
//   - VerifyToken:         Webhook challenge token (required to receive updates).
//   - WebhookEndpoint:     HTTP path to serve the webhook on (default: "/webhook").
//   - APIVersion:          Graph API version (default: "22.0").
//   - HTTPClient:          Custom *http.Client (nil → 30 s timeout default).
//   - FilterUpdates:       Drop updates not belonging to this PhoneID (default true).
//   - ContinueHandling:    Call all matching handlers, not just the first (default false).
//   - ValidateUpdates:     Verify HMAC signature on every update (default true).
//   - BusinessPrivateKey:  RSA private key PEM for Flow decryption (optional).
//   - BusinessPrivateKeyPassword: Password for BusinessPrivateKey (optional).
type Config struct {
	Token                      string
	PhoneID                    string
	BusinessAccountID          string
	AppID                      string
	AppSecret                  string
	VerifyToken                string
	WebhookEndpoint            string
	APIVersion                 string
	HTTPClient                 *http.Client
	FilterUpdates              bool
	ContinueHandling           bool
	ValidateUpdates            bool
	BusinessPrivateKey         string
	BusinessPrivateKeyPassword string
}

// ────────────────────────────────────────────────────────────────────────────────
// WhatsApp client
// Mirrors pywa's WhatsApp class.
// ────────────────────────────────────────────────────────────────────────────────

// WhatsApp is the main entry point for all gowa operations: sending messages,
// managing templates/flows, registering webhook handlers, and more.
//
// Create one with New() or NewWithConfig() and then call its methods.
//
// Example — without webhook (API-only):
//
//	wa, err := gowa.New("PHONE_ID", "TOKEN")
//	if err != nil { log.Fatal(err) }
//	msg, err := wa.SendMessage("1234567890", "Hello from gowa!")
//
// Example — with webhook (embedded in any net/http server):
//
//	wa, err := gowa.NewWithConfig(gowa.Config{
//	    Token:           "TOKEN",
//	    PhoneID:         "PHONE_ID",
//	    AppSecret:       "APP_SECRET",
//	    VerifyToken:     "MY_VERIFY_TOKEN",
//	    WebhookEndpoint: "/webhook",
//	})
//	wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
//	    msg.Reply("Hi!")
//	}, gowa.FilterText)
//
//	http.ListenAndServe(":8080", wa.Handler())
type WhatsApp struct {
	phoneID           string
	businessAccountID string
	appID             string
	appSecret         string
	verifyToken       string
	webhookEndpoint   string
	filterUpdates     bool
	continueHandling  bool
	validateUpdates   bool
	privateKey        string
	privateKeyPwd     string

	api   *graphAPI
	hdlrs handlers

	// Flow endpoint → handler mapping
	flowEndpoints map[string]FlowRequestHandlerFunc
	mu            sync.RWMutex

	// Duplicate-update deduplication (maps message ID → seen timestamp)
	seen   map[string]time.Time
	seenMu sync.Mutex

	// Active blocking listeners (from wa.Listen calls)
	listeners map[ListenerKey]*listenerEntry
}

// New creates a WhatsApp client with minimal required settings.
// Use this when you only need to send messages and do not need a webhook.
//
// Parameters:
//   - phoneID: the phone number ID to send messages from.
//   - token:   the access token.
//
// Returns:
//   - *WhatsApp ready to send messages.
//   - error if phoneID or token is empty.
func New(phoneID, token string) (*WhatsApp, error) {
	return NewWithConfig(Config{
		PhoneID:       phoneID,
		Token:         token,
		FilterUpdates: true,
	})
}

// NewWithConfig creates a WhatsApp client from a full Config struct.
//
// Parameters:
//   - cfg: Config with all desired settings.
//
// Returns:
//   - *WhatsApp.
//   - error if required fields are missing.
func NewWithConfig(cfg Config) (*WhatsApp, error) {
	if cfg.WebhookEndpoint == "" {
		cfg.WebhookEndpoint = "/webhook"
	}
	if cfg.APIVersion == "" {
		cfg.APIVersion = defaultAPIVersion
	}

	wa := &WhatsApp{
		phoneID:           cfg.PhoneID,
		businessAccountID: cfg.BusinessAccountID,
		appID:             cfg.AppID,
		appSecret:         cfg.AppSecret,
		verifyToken:       cfg.VerifyToken,
		webhookEndpoint:   cfg.WebhookEndpoint,
		filterUpdates:     cfg.FilterUpdates,
		continueHandling:  cfg.ContinueHandling,
		validateUpdates:   cfg.ValidateUpdates,
		privateKey:        cfg.BusinessPrivateKey,
		privateKeyPwd:     cfg.BusinessPrivateKeyPassword,
		seen:              make(map[string]time.Time),
	}

	if cfg.Token != "" {
		wa.api = newGraphAPI(cfg.Token, cfg.HTTPClient, cfg.APIVersion)
	}

	return wa, nil
}

// PhoneID returns the sender phone number ID.
func (wa *WhatsApp) PhoneID() string { return wa.phoneID }

// BusinessAccountID returns the WABA ID.
func (wa *WhatsApp) BusinessAccountID() string { return wa.businessAccountID }

// SetToken updates the bearer token used for API requests.
// Thread-safe.
//
// Parameters:
//   - token: new bearer token.
func (wa *WhatsApp) SetToken(token string) {
	if wa.api != nil {
		wa.api.token = token
	}
}

// requireAPI returns an error if no API client was configured
// (i.e. the WhatsApp was created without a token).
func (wa *WhatsApp) requireAPI() error {
	if wa.api == nil {
		return fmt.Errorf("gowa: no token configured — provide a token via Config.Token to call API methods")
	}
	return nil
}

// resolveSender returns the effective sender phone ID.
// If sender is empty, falls back to wa.phoneID.
//
// Returns:
//   - sender phone ID string.
//   - error if both are empty.
func (wa *WhatsApp) resolveSender(sender string) (string, error) {
	if sender != "" {
		return sender, nil
	}
	if wa.phoneID != "" {
		return wa.phoneID, nil
	}
	return "", fmt.Errorf("gowa: no phone_id configured — set PhoneID in Config or pass sender explicitly")
}

// resolveWABAID returns the effective WABA ID.
//
// Returns:
//   - WABA ID string.
//   - error if both wabaID arg and wa.businessAccountID are empty.
func (wa *WhatsApp) resolveWABAID(wabaID string) (string, error) {
	if wabaID != "" {
		return wabaID, nil
	}
	if wa.businessAccountID != "" {
		return wa.businessAccountID, nil
	}
	return "", fmt.Errorf("gowa: no business_account_id configured — set BusinessAccountID in Config or pass waba_id explicitly")
}

// extractSentMessage builds a SentMessage from a /messages API response.
func extractSentMessage(res map[string]any, senderPhoneID, to string) *SentMessage {
	id := ""
	if msgs, ok := res["messages"].([]any); ok && len(msgs) > 0 {
		if m, ok := msgs[0].(map[string]any); ok {
			id = toString(m["id"])
		}
	}
	return &SentMessage{
		ID:          id,
		FromPhoneID: senderPhoneID,
		To:          to,
		Timestamp:   time.Now().UTC(),
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// SECTION: Sending messages
// All methods mirror pywa's WhatsApp.send_* methods.
// ════════════════════════════════════════════════════════════════════════════════

// ── SendMessage options ────────────────────────────────────────────────────────

// SendMessageOptions configures optional fields for SendMessage.
type SendMessageOptions struct {
	// PreviewURL enables link-preview rendering for the first URL in the body.
	PreviewURL bool
	// ReplyToMessageID quotes a previous message.
	ReplyToMessageID string
	// Tracker is an opaque string (≤512 chars) returned in the MessageStatus webhook.
	Tracker string
	// IdentityKeyHash drops the message if the recipient identity hash mismatches.
	IdentityKeyHash string
	// Sender overrides the client's default phone ID for this request.
	Sender string
	// Header is an optional header text when sending with buttons.
	Header string
	// Footer is an optional footer text when sending with buttons.
	Footer string
	// Buttons attaches interactive buttons to the message.
	Buttons any // *SectionList | []Button | *URLButton | *FlowButton
}

// SendMessage sends a text message, optionally with interactive buttons.
// Mirrors pywa's WhatsApp.send_message.
//
// Parameters:
//   - to:   recipient WhatsApp ID (phone number with country code).
//   - text: message body (markdown allowed, max 4096 chars).
//   - opts: optional SendMessageOptions.
//
// Returns:
//   - *SentMessage with the message ID.
//   - error (*WhatsAppError on API failure).
func (wa *WhatsApp) SendMessage(to, text string, opts ...SendMessageOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendMessageOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	var payload sendMessagePayload
	payload.To = to
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	payload.BizOpaqueCallbackData = o.Tracker
	payload.RecipientIdentityKeyHash = o.IdentityKeyHash

	if o.Buttons == nil {
		payload.Type = "text"
		payload.Text = map[string]any{"body": text, "preview_url": o.PreviewURL}
	} else {
		// Interactive message with buttons
		payload.Type = "interactive"
		payload.Interactive = buildInteractivePayload(text, o.Header, o.Footer, o.Buttons)
	}

	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, to), nil
}

// SendText is an alias for SendMessage.
var _ = (*WhatsApp).SendText // ensure method exists

// SendText is an alias for SendMessage for symmetry with pywa's send_text alias.
//
// Parameters:
//   - to:   recipient WA ID.
//   - text: message body.
//   - opts: optional SendMessageOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) SendText(to, text string, opts ...SendMessageOptions) (*SentMessage, error) {
	return wa.SendMessage(to, text, opts...)
}

// ── SendMediaOptions ──────────────────────────────────────────────────────────

// SendMediaOptions configures optional parameters for all send-media methods.
type SendMediaOptions struct {
	// ReplyToMessageID quotes a previous message.
	ReplyToMessageID string
	// Tracker is an opaque callback data string (≤512 chars).
	Tracker string
	// IdentityKeyHash drops delivery if the hash mismatches.
	IdentityKeyHash string
	// Sender overrides the default phone ID.
	Sender string
	// Footer is displayed below the caption when buttons are attached.
	Footer string
	// Buttons attaches interactive buttons.
	Buttons any
	// MimeType overrides MIME-type detection (needed for raw bytes without extension).
	MimeType string
	// Filename sets the document filename displayed in WhatsApp.
	Filename string
}

// resolveMedia prepares a media parameter for inclusion in the API payload.
// It accepts: URL string, local file path, or raw []byte.
//
// Parameters:
//   - media:    URL string / file path string / raw []byte.
//   - mimeType: explicit MIME type override (may be empty).
//   - sender:   phone ID to upload to when uploading raw bytes.
//   - mediaType: "image" | "video" | "audio" | "document" | "sticker".
//
// Returns:
//   - mediaMap: API sub-object {"link": url} or {"id": mediaID}.
//   - mediaID:  non-empty when media was uploaded (so caller can return it).
//   - error.
func (wa *WhatsApp) resolveMedia(media any, mimeType, sender, mediaType string) (map[string]any, string, error) {
	switch v := media.(type) {
	case string:
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			return map[string]any{"link": v}, "", nil
		}
		// Treat as file path
		data, err := os.ReadFile(v)
		if err != nil {
			return nil, "", fmt.Errorf("reading media file %q: %w", v, err)
		}
		if mimeType == "" {
			mimeType = mimeTypeFromPath(v)
		}
		filename := filepath.Base(v)
		mediaID, err := wa.api.uploadMedia(sender, data, mimeType, filename)
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"id": mediaID}, mediaID, nil
	case []byte:
		if mimeType == "" {
			return nil, "", fmt.Errorf("mime_type is required when sending raw bytes")
		}
		ext, _ := extensionFromMimeType(mimeType)
		filename := mediaType + ext
		mediaID, err := wa.api.uploadMedia(sender, v, mimeType, filename)
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"id": mediaID}, mediaID, nil
	case io.Reader:
		data, err := io.ReadAll(v)
		if err != nil {
			return nil, "", fmt.Errorf("reading media reader: %w", err)
		}
		if mimeType == "" {
			return nil, "", fmt.Errorf("mime_type is required when sending an io.Reader")
		}
		ext, _ := extensionFromMimeType(mimeType)
		mediaID, err := wa.api.uploadMedia(sender, data, mimeType, mediaType+ext)
		if err != nil {
			return nil, "", err
		}
		return map[string]any{"id": mediaID}, mediaID, nil
	default:
		return nil, "", fmt.Errorf("unsupported media type %T; pass a URL string, file path string, or []byte", media)
	}
}

// SendImage sends an image message.
// Mirrors pywa's WhatsApp.send_image.
//
// Parameters:
//   - to:      recipient WA ID.
//   - image:   URL string, file path string, or []byte.
//   - caption: optional caption text.
//   - opts:    optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (wa *WhatsApp) SendImage(to string, image any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	return wa.sendMedia(to, "image", image, caption, opts...)
}

// SendVideo sends a video message.
// Mirrors pywa's WhatsApp.send_video.
//
// Parameters:
//   - to:      recipient WA ID.
//   - video:   URL string, file path string, or []byte.
//   - caption: optional caption.
//   - opts:    optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (wa *WhatsApp) SendVideo(to string, video any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	return wa.sendMedia(to, "video", video, caption, opts...)
}

// SendDocument sends a document message.
// Mirrors pywa's WhatsApp.send_document.
//
// Parameters:
//   - to:       recipient WA ID.
//   - document: URL string, file path string, or []byte.
//   - caption:  optional caption.
//   - opts:     optional SendMediaOptions (set Filename for a custom filename).
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (wa *WhatsApp) SendDocument(to string, document any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	return wa.sendMedia(to, "document", document, caption, opts...)
}

// SendAudio sends an audio message.
// Mirrors pywa's WhatsApp.send_audio.
//
// Parameters:
//   - to:    recipient WA ID.
//   - audio: URL string, file path string, or []byte.
//   - opts:  optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (wa *WhatsApp) SendAudio(to string, audio any, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	return wa.sendMedia(to, "audio", audio, "", opts...)
}

// SendVoice sends a voice note (OGG/OPUS encoded).
// Mirrors pywa's WhatsApp.send_voice.
//
// Parameters:
//   - to:    recipient WA ID.
//   - voice: URL string, file path string, or []byte (must be OGG/OPUS).
//   - opts:  optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (wa *WhatsApp) SendVoice(to string, voice any, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	o := SendMediaOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.MimeType == "" {
		o.MimeType = "audio/ogg; codecs=opus"
	}
	return wa.sendMedia(to, "audio", voice, "", o)
}

// SendSticker sends a static or animated sticker.
// Mirrors pywa's WhatsApp.send_sticker.
//
// Parameters:
//   - to:      recipient WA ID.
//   - sticker: URL string, file path string, or []byte (WebP format).
//   - opts:    optional SendMediaOptions.
//
// Returns:
//   - *SentMediaMessage.
//   - error.
func (wa *WhatsApp) SendSticker(to string, sticker any, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	return wa.sendMedia(to, "sticker", sticker, "", opts...)
}

// sendMedia is the shared implementation for all send-media methods.
func (wa *WhatsApp) sendMedia(to, mediaType string, media any, caption string, opts ...SendMediaOptions) (*SentMediaMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendMediaOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	mediaMap, mediaID, err := wa.resolveMedia(media, o.MimeType, sender, mediaType)
	if err != nil {
		return nil, err
	}
	if caption != "" {
		mediaMap["caption"] = caption
	}
	if o.Filename != "" && mediaType == "document" {
		mediaMap["filename"] = o.Filename
	}

	payload := sendMessagePayload{To: to, Type: mediaType}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	payload.BizOpaqueCallbackData = o.Tracker
	payload.RecipientIdentityKeyHash = o.IdentityKeyHash

	// Assign media map to the correct type field
	switch mediaType {
	case "image":
		payload.Image = mediaMap
	case "video":
		payload.Video = mediaMap
	case "audio":
		payload.Audio = mediaMap
	case "document":
		payload.Document = mediaMap
	case "sticker":
		payload.Sticker = mediaMap
	}

	if o.Buttons != nil {
		if caption == "" {
			return nil, fmt.Errorf("gowa: a caption is required when sending %s with buttons", mediaType)
		}
		payload.Type = "interactive"
		header := map[string]any{"type": mediaType, mediaType: mediaMap}
		payload.Interactive = buildInteractivePayloadWithHeader(caption, o.Footer, o.Buttons, header)
	}

	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	sm := extractSentMessage(res, sender, to)
	return &SentMediaMessage{SentMessage: *sm, MediaID: mediaID}, nil
}

// ── SendReactionOptions / SendReaction / RemoveReaction ───────────────────────

// SendReactionOptions holds optional parameters for SendReaction.
type SendReactionOptions struct {
	Tracker         string
	IdentityKeyHash string
	Sender          string
}

// SendReaction sends an emoji reaction to a message.
// Mirrors pywa's WhatsApp.send_reaction.
//
// Parameters:
//   - to:        recipient WA ID.
//   - emoji:     the emoji string.
//   - messageID: the wamid of the message to react to.
//   - opts:      optional SendReactionOptions.
//
// Returns:
//   - *SentReaction.
//   - error.
func (wa *WhatsApp) SendReaction(to, emoji, messageID string, opts ...SendReactionOptions) (*SentReaction, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendReactionOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}
	payload := sendMessagePayload{
		To:   to,
		Type: "reaction",
		Reaction: map[string]any{
			"emoji":      emoji,
			"message_id": messageID,
		},
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	sm := extractSentMessage(res, sender, to)
	return &SentReaction{SentMessage: *sm, ReactedToMessageID: messageID}, nil
}

// RemoveReactionOptions holds optional parameters for RemoveReaction.
type RemoveReactionOptions = SendReactionOptions

// RemoveReaction removes an emoji reaction from a message.
// Mirrors pywa's WhatsApp.remove_reaction.
//
// Parameters:
//   - to:        recipient WA ID.
//   - messageID: the wamid of the message to un-react.
//   - opts:      optional RemoveReactionOptions.
//
// Returns:
//   - *SentReaction.
//   - error.
func (wa *WhatsApp) RemoveReaction(to, messageID string, opts ...RemoveReactionOptions) (*SentReaction, error) {
	return wa.SendReaction(to, "", messageID, opts...)
}

// ── Location ──────────────────────────────────────────────────────────────────

// SendLocationOptions holds optional parameters for SendLocation.
type SendLocationOptions struct {
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
}

// SendLocation sends a geographic location.
// Mirrors pywa's WhatsApp.send_location.
//
// Parameters:
//   - to:        recipient WA ID.
//   - latitude:  decimal latitude.
//   - longitude: decimal longitude.
//   - name:      optional location name.
//   - address:   optional address text.
//   - opts:      optional SendLocationOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) SendLocation(to string, latitude, longitude float64, name, address string, opts ...SendLocationOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendLocationOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}
	payload := sendMessagePayload{
		To:   to,
		Type: "location",
		Location: map[string]any{
			"latitude":  latitude,
			"longitude": longitude,
			"name":      name,
			"address":   address,
		},
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, to), nil
}

// RequestLocation sends an interactive location-request message.
// Mirrors pywa's WhatsApp.request_location.
//
// Parameters:
//   - to:   recipient WA ID.
//   - text: body text shown with the "Send location" button.
//   - opts: optional SendLocationOptions.
//
// Returns:
//   - *SentLocationRequest.
//   - error.
func (wa *WhatsApp) RequestLocation(to, text string, opts ...SendLocationOptions) (*SentLocationRequest, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendLocationOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}
	payload := sendMessagePayload{
		To:   to,
		Type: "interactive",
		Interactive: map[string]any{
			"type":   "location_request_message",
			"body":   map[string]any{"text": text},
			"action": map[string]any{"name": "send_location"},
		},
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	sm := extractSentMessage(res, sender, to)
	return &SentLocationRequest{SentMessage: *sm}, nil
}

// ── Contact ───────────────────────────────────────────────────────────────────

// SendContactOptions holds optional parameters for SendContact.
type SendContactOptions struct {
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
}

// SendContact sends one or more rich contact cards.
// Mirrors pywa's WhatsApp.send_contact.
//
// Parameters:
//   - to:       recipient WA ID.
//   - contacts: one or more Contact values to send (up to 257).
//   - opts:     optional SendContactOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) SendContact(to string, contacts []Contact, opts ...SendContactOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendContactOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	dicts := make([]map[string]any, len(contacts))
	for i, c := range contacts {
		dicts[i] = c.toDict()
	}

	payload := sendMessagePayload{
		To:                       to,
		Type:                     "contacts",
		Contacts:                 dicts,
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, to), nil
}

// ── Template ──────────────────────────────────────────────────────────────────

// SendTemplateOptions holds optional parameters for SendTemplate.
type SendTemplateOptions struct {
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
	// Params are the component parameters to inject into the template.
	// Build them with TemplateParam helpers.
	Params []map[string]any
}

// SendTemplate sends a pre-approved message template.
// Mirrors pywa's WhatsApp.send_template.
//
// Parameters:
//   - to:       recipient WA ID.
//   - name:     template name.
//   - language: BCP-47 language code (e.g. "en_US").
//   - opts:     optional SendTemplateOptions.
//
// Returns:
//   - *SentTemplate.
//   - error.
func (wa *WhatsApp) SendTemplate(to, name, language string, opts ...SendTemplateOptions) (*SentTemplate, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendTemplateOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	tmpl := map[string]any{
		"name":     name,
		"language": map[string]any{"code": language},
	}
	if len(o.Params) > 0 {
		tmpl["components"] = o.Params
	}

	payload := sendMessagePayload{
		To:                       to,
		Type:                     "template",
		Template:                 tmpl,
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	sm := extractSentMessage(res, sender, to)
	return &SentTemplate{SentMessage: *sm}, nil
}

// ── Catalog / product messages ────────────────────────────────────────────────

// SendCatalogOptions holds optional parameters for SendCatalog.
type SendCatalogOptions struct {
	ThumbnailProductSKU string
	ReplyToMessageID    string
	Tracker             string
	IdentityKeyHash     string
	Sender              string
}

// SendCatalog sends a catalog message linking to the business's full product catalog.
// Mirrors pywa's WhatsApp.send_catalog.
//
// Parameters:
//   - to:     recipient WA ID.
//   - body:   body text (up to 1024 chars).
//   - footer: optional footer (up to 60 chars).
//   - opts:   optional SendCatalogOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) SendCatalog(to, body, footer string, opts ...SendCatalogOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendCatalogOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	action := map[string]any{"name": "catalog_message"}
	if o.ThumbnailProductSKU != "" {
		action["parameters"] = map[string]any{
			"thumbnail_product_retailer_id": o.ThumbnailProductSKU,
		}
	}
	interactive := map[string]any{
		"type":   "catalog_message",
		"body":   map[string]any{"text": body},
		"action": action,
	}
	if footer != "" {
		interactive["footer"] = map[string]any{"text": footer}
	}

	payload := sendMessagePayload{
		To:                       to,
		Type:                     "interactive",
		Interactive:              interactive,
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, to), nil
}

// SendProductOptions holds optional parameters for SendProduct.
type SendProductOptions struct {
	Body             string
	Footer           string
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
}

// SendProduct sends a single product from a catalog.
// Mirrors pywa's WhatsApp.send_product.
//
// Parameters:
//   - to:        recipient WA ID.
//   - catalogID: the catalog ID.
//   - sku:       product retailer SKU.
//   - opts:      optional SendProductOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) SendProduct(to, catalogID, sku string, opts ...SendProductOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendProductOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	interactive := map[string]any{
		"type": "product",
		"action": map[string]any{
			"catalog_id":          catalogID,
			"product_retailer_id": sku,
		},
	}
	if o.Body != "" {
		interactive["body"] = map[string]any{"text": o.Body}
	}
	if o.Footer != "" {
		interactive["footer"] = map[string]any{"text": o.Footer}
	}

	payload := sendMessagePayload{
		To:                       to,
		Type:                     "interactive",
		Interactive:              interactive,
		BizOpaqueCallbackData:    o.Tracker,
		RecipientIdentityKeyHash: o.IdentityKeyHash,
	}
	if o.ReplyToMessageID != "" {
		payload.Context = map[string]any{"message_id": o.ReplyToMessageID}
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, to), nil
}

// ── Read receipts / typing ────────────────────────────────────────────────────

// MarkMessageAsRead marks a message as read and stops delivery retries.
// Mirrors pywa's WhatsApp.mark_message_as_read.
//
// Parameters:
//   - messageID: the wamid of the message.
//   - sender:    optional sender phone ID override.
//
// Returns:
//   - error if the API call fails.
func (wa *WhatsApp) MarkMessageAsRead(messageID, sender string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	phoneID, err := wa.resolveSender(sender)
	if err != nil {
		return err
	}
	return wa.api.markMessageAsRead(phoneID, messageID)
}

// IndicateTyping marks a message as read and shows the typing indicator.
// Mirrors pywa's WhatsApp.indicate_typing.
//
// Parameters:
//   - messageID: the wamid of the message.
//   - sender:    optional sender phone ID override.
//
// Returns:
//   - error if the API call fails.
func (wa *WhatsApp) IndicateTyping(messageID, sender string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	phoneID, err := wa.resolveSender(sender)
	if err != nil {
		return err
	}
	return wa.api.indicateTyping(phoneID, messageID)
}

// ── Media ─────────────────────────────────────────────────────────────────────

// UploadMedia uploads a media file to WhatsApp servers and returns its media ID.
// Mirrors pywa's WhatsApp.upload_media.
//
// Parameters:
//   - media:     URL string, file path string, or []byte.
//   - mimeType:  MIME type (required when passing []byte).
//   - filename:  optional filename hint.
//   - phoneID:   optional override for the upload destination phone ID.
//
// Returns:
//   - mediaID string.
//   - error.
func (wa *WhatsApp) UploadMedia(media any, mimeType, filename, phoneID string) (string, error) {
	if err := wa.requireAPI(); err != nil {
		return "", err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return "", err
	}

	var data []byte
	switch v := media.(type) {
	case string:
		if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
			var dlErr error
			data, dlErr = wa.api.downloadMedia(v)
			if dlErr != nil {
				return "", dlErr
			}
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
		} else {
			var readErr error
			data, readErr = os.ReadFile(v)
			if readErr != nil {
				return "", readErr
			}
			if mimeType == "" {
				mimeType = mimeTypeFromPath(v)
			}
			if filename == "" {
				filename = filepath.Base(v)
			}
		}
	case []byte:
		data = v
		if mimeType == "" {
			return "", fmt.Errorf("mime_type is required when passing []byte to UploadMedia")
		}
	default:
		return "", fmt.Errorf("UploadMedia: unsupported media type %T", media)
	}

	if filename == "" {
		ext, _ := extensionFromMimeType(mimeType)
		filename = "upload" + ext
	}
	return wa.api.uploadMedia(pid, data, mimeType, filename)
}

// GetMediaURL returns a short-lived (5 min) download URL for a media ID.
// Mirrors pywa's WhatsApp.get_media_url.
//
// Parameters:
//   - mediaID: the WhatsApp media ID.
//
// Returns:
//   - *MediaURL containing the URL and metadata.
//   - error.
func (wa *WhatsApp) GetMediaURL(mediaID string) (*MediaURL, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.getMediaURL(mediaID)
	if err != nil {
		return nil, err
	}
	return &MediaURL{
		ID:       toString(res["id"]),
		URL:      toString(res["url"]),
		MimeType: toString(res["mime_type"]),
		SHA256:   toString(res["sha256"]),
		FileSize: int64(toFloat64(res["file_size"])),
		client:   wa,
	}, nil
}

// DownloadMedia downloads a media file to disk.
// Mirrors pywa's WhatsApp.download_media.
//
// Parameters:
//   - mediaURL: the URL from GetMediaURL.
//   - destPath: directory or full file path to write to.
//
// Returns:
//   - full path to the saved file.
//   - error.
func (wa *WhatsApp) DownloadMedia(mediaURL, destPath string) (string, error) {
	if err := wa.requireAPI(); err != nil {
		return "", err
	}
	data, err := wa.api.downloadMedia(mediaURL)
	if err != nil {
		return "", err
	}
	// If destPath is a directory, use a hash-based filename
	info, statErr := os.Stat(destPath)
	if statErr == nil && info.IsDir() {
		destPath = filepath.Join(destPath, fmt.Sprintf("%x", time.Now().UnixNano()))
	}
	if err := os.WriteFile(destPath, data, 0o644); err != nil {
		return "", err
	}
	return destPath, nil
}

// GetMediaBytes downloads a media file and returns the raw bytes.
// Mirrors pywa's WhatsApp.get_media_bytes.
//
// Parameters:
//   - mediaURL: the URL from GetMediaURL.
//
// Returns:
//   - []byte with the file content.
//   - error.
func (wa *WhatsApp) GetMediaBytes(mediaURL string) ([]byte, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	return wa.api.downloadMedia(mediaURL)
}

// DeleteMedia deletes an uploaded media file from WhatsApp servers.
// Mirrors pywa's WhatsApp.delete_media.
//
// Parameters:
//   - mediaID: the media ID to delete.
//   - phoneID: optional phone ID filter (empty to skip).
//
// Returns:
//   - error.
func (wa *WhatsApp) DeleteMedia(mediaID, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	return wa.api.deleteMedia(mediaID, phoneID)
}

// ── Business profile ──────────────────────────────────────────────────────────

// GetBusinessProfile fetches the business profile.
// Mirrors pywa's WhatsApp.get_business_profile.
//
// Parameters:
//   - phoneID: optional override (uses Config.PhoneID if empty).
//
// Returns:
//   - *BusinessProfile.
//   - error.
func (wa *WhatsApp) GetBusinessProfile(phoneID string) (*BusinessProfile, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	fields := "about,address,description,email,profile_picture_url,websites,vertical"
	res, err := wa.api.getBusinessProfile(pid, fields)
	if err != nil {
		return nil, err
	}
	data := toAnySlice(res["data"])
	if len(data) == 0 {
		return nil, fmt.Errorf("gowa: no business profile returned")
	}
	dm := toMap(data[0])
	bp := &BusinessProfile{
		About:        toString(dm["about"]),
		Address:      toString(dm["address"]),
		Description:  toString(dm["description"]),
		Email:        toString(dm["email"]),
		VerticalName: toString(dm["vertical"]),
	}
	for _, w := range toAnySlice(dm["websites"]) {
		bp.Websites = append(bp.Websites, fmt.Sprint(w))
	}
	return bp, nil
}

// UpdateBusinessProfileOptions holds the fields for UpdateBusinessProfile.
type UpdateBusinessProfileOptions struct {
	About        *string
	Address      *string
	Description  *string
	Email        *string
	Websites     []string
	VerticalName *string
	PhoneID      string
}

// UpdateBusinessProfile updates one or more fields of the business profile.
// Mirrors pywa's WhatsApp.update_business_profile.
//
// Parameters:
//   - opts: fields to update (nil fields are not sent).
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateBusinessProfile(opts UpdateBusinessProfileOptions) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(opts.PhoneID)
	if err != nil {
		return err
	}
	data := map[string]any{}
	if opts.About != nil {
		data["about"] = *opts.About
	}
	if opts.Address != nil {
		data["address"] = *opts.Address
	}
	if opts.Description != nil {
		data["description"] = *opts.Description
	}
	if opts.Email != nil {
		data["email"] = *opts.Email
	}
	if opts.Websites != nil {
		data["websites"] = opts.Websites
	}
	if opts.VerticalName != nil {
		data["vertical"] = *opts.VerticalName
	}
	return wa.api.updateBusinessProfile(pid, data)
}

// ── Templates ─────────────────────────────────────────────────────────────────

// CreateTemplateOptions holds parameters for CreateTemplate.
type CreateTemplateOptions struct {
	// WABAOID overrides the client WABA ID.
	WABAOID string
}

// CreateTemplate creates a new message template.
// Mirrors pywa's WhatsApp.create_template.
//
// Parameters:
//   - template: map[string]any JSON-ready template definition.
//   - opts:     optional CreateTemplateOptions.
//
// Returns:
//   - *CreatedTemplate.
//   - error.
func (wa *WhatsApp) CreateTemplate(template map[string]any, opts ...CreateTemplateOptions) (*CreatedTemplate, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wabaID := ""
	if len(opts) > 0 {
		wabaID = opts[0].WABAOID
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.createTemplate(wid, template)
	if err != nil {
		return nil, err
	}
	return &CreatedTemplate{
		ID:       toString(res["id"]),
		Status:   TemplateStatus(toString(res["status"])),
		Category: TemplateCategory(toString(res["category"])),
	}, nil
}

// DeleteTemplate deletes a template by name (and optionally by ID).
// Mirrors pywa's WhatsApp.delete_template.
//
// Parameters:
//   - templateName: template name.
//   - templateID:   optional; only the matching ID will be deleted if provided.
//   - wabaID:       optional WABA ID override.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeleteTemplate(templateName, templateID, wabaID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return err
	}
	return wa.api.deleteTemplate(wid, templateName, templateID)
}

// ── Flow management ───────────────────────────────────────────────────────────

// CreateFlowOptions holds optional parameters for CreateFlow.
type CreateFlowOptions struct {
	WABAOID     string
	EndpointURI string
	CloneFlowID string
}

// CreateFlow creates a new WhatsApp Flow.
// Mirrors pywa's WhatsApp.create_flow.
//
// Parameters:
//   - name:       unique flow name.
//   - categories: one or more FlowCategory values.
//   - opts:       optional CreateFlowOptions.
//
// Returns:
//   - *CreatedFlow with the new flow ID.
//   - error.
func (wa *WhatsApp) CreateFlow(name string, categories []FlowCategory, opts ...CreateFlowOptions) (*CreatedFlow, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := CreateFlowOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	wid, err := wa.resolveWABAID(o.WABAOID)
	if err != nil {
		return nil, err
	}
	cats := make([]string, len(categories))
	for i, c := range categories {
		cats[i] = string(c)
	}
	res, err := wa.api.createFlow(wid, name, cats)
	if err != nil {
		return nil, err
	}
	return &CreatedFlow{ID: toString(res["id"])}, nil
}

// PublishFlow publishes a flow (irreversible).
// Mirrors pywa's WhatsApp.publish_flow.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - error.
func (wa *WhatsApp) PublishFlow(flowID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	return wa.api.publishFlow(flowID)
}

// DeleteFlow deletes a DRAFT flow.
// Mirrors pywa's WhatsApp.delete_flow.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeleteFlow(flowID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	return wa.api.deleteFlow(flowID)
}

// DeprecateFlow marks a published flow as deprecated.
// Mirrors pywa's WhatsApp.deprecate_flow.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeprecateFlow(flowID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	return wa.api.deprecateFlow(flowID)
}

// GetFlow fetches the metadata of a single flow.
// Mirrors pywa's WhatsApp.get_flow.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - *FlowDetails.
//   - error.
func (wa *WhatsApp) GetFlow(flowID string) (*FlowDetails, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	fields := "id,name,status,categories,validation_errors,endpoint_uri,preview"
	res, err := wa.api.getFlow(flowID, fields)
	if err != nil {
		return nil, err
	}
	return parseFlowDetails(res), nil
}

// GetFlows returns all flows for the configured WABA.
// Mirrors pywa's WhatsApp.get_flows.
//
// Parameters:
//   - wabaID:     optional WABA ID override.
//   - pagination: optional cursor paging.
//
// Returns:
//   - *Result[*FlowDetails].
//   - error.
func (wa *WhatsApp) GetFlows(wabaID string, pagination *Pagination) (*Result[*FlowDetails], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	fields := "id,name,status,categories,validation_errors,endpoint_uri"
	pg := map[string]string{}
	if pagination != nil {
		pg = pagination.toDict()
	}
	res, err := wa.api.getFlows(wid, fields, pg)
	if err != nil {
		return nil, err
	}
	return parseFlowsResult(wa, res, wid, pagination), nil
}

func parseFlowDetails(res map[string]any) *FlowDetails {
	fd := &FlowDetails{
		ID:          toString(res["id"]),
		Name:        toString(res["name"]),
		Status:      FlowStatus(toString(res["status"])),
		EndpointURI: toString(res["endpoint_uri"]),
	}
	for _, cat := range toAnySlice(res["categories"]) {
		fd.Categories = append(fd.Categories, FlowCategory(fmt.Sprint(cat)))
	}
	if preview := toMap(res["preview"]); preview != nil {
		fd.PreviewURL = toString(preview["preview_url"])
	}
	return fd
}

func parseFlowsResult(wa *WhatsApp, res map[string]any, wabaID string, pg *Pagination) *Result[*FlowDetails] {
	r := &Result[*FlowDetails]{}
	for _, d := range toAnySlice(res["data"]) {
		if dm := toMap(d); dm != nil {
			r.Items = append(r.Items, parseFlowDetails(dm))
		}
	}
	if paging := toMap(res["paging"]); paging != nil {
		if cursors := toMap(paging["cursors"]); cursors != nil {
			r.NextCursor = toString(cursors["after"])
			r.PrevCursor = toString(cursors["before"])
		}
	}
	if r.NextCursor != "" {
		r.fetchFn = func(after string) (*Result[*FlowDetails], error) {
			return wa.GetFlows(wabaID, &Pagination{After: after})
		}
	}
	return r
}

// ── QR codes ──────────────────────────────────────────────────────────────────

// CreateQRCode creates a QR code for a prefilled message.
// Mirrors pywa's WhatsApp.create_qr_code.
//
// Parameters:
//   - prefilledMessage: text embedded in the QR code.
//   - imageType:        "PNG" or "SVG".
//   - phoneID:          optional phone ID override.
//
// Returns:
//   - *QRCode.
//   - error.
func (wa *WhatsApp) CreateQRCode(prefilledMessage, imageType, phoneID string) (*QRCode, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	if imageType == "" {
		imageType = "PNG"
	}
	res, err := wa.api.createQRCode(pid, prefilledMessage, imageType)
	if err != nil {
		return nil, err
	}
	return &QRCode{
		Code:             toString(res["code"]),
		PrefilledMessage: toString(res["prefilled_message"]),
		DeepLinkURL:      toString(res["deep_link_url"]),
		QRImageURL:       toString(res["qr_image_url"]),
		client:           wa,
		phoneID:          pid,
	}, nil
}

// DeleteQRCode deletes a QR code.
// Mirrors pywa's WhatsApp.delete_qr_code.
//
// Parameters:
//   - code:    QR code identifier.
//   - phoneID: optional phone ID override.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeleteQRCode(code string, phoneID ...string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid := ""
	if len(phoneID) > 0 {
		pid = phoneID[0]
	}
	pid, err := wa.resolveSender(pid)
	if err != nil {
		return err
	}
	return wa.api.deleteQRCode(pid, code)
}

// ── User blocking ─────────────────────────────────────────────────────────────

// BlockUsers blocks one or more users from messaging the business.
// Mirrors pywa's WhatsApp.block_users.
//
// Parameters:
//   - users:   slice of WA IDs (phone numbers with country code, no '+').
//   - phoneID: optional phone ID override.
//
// Returns:
//   - *UsersBlockedResult.
//   - error.
func (wa *WhatsApp) BlockUsers(users []string, phoneID ...string) (*UsersBlockedResult, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid := ""
	if len(phoneID) > 0 {
		pid = phoneID[0]
	}
	pid, err := wa.resolveSender(pid)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.blockUsers(pid, users)
	if err != nil {
		return nil, err
	}
	result := &UsersBlockedResult{}
	for _, u := range toAnySlice(res["added_users"]) {
		um := toMap(u)
		result.AddedUsers = append(result.AddedUsers, BlockedUser{
			WAID:  toString(um["wa_id"]),
			Input: toString(um["input"]),
		})
	}
	for _, u := range toAnySlice(res["failed_users"]) {
		um := toMap(u)
		result.FailedUsers = append(result.FailedUsers, BlockedUser{
			WAID:  toString(um["wa_id"]),
			Input: toString(um["input"]),
		})
	}
	return result, nil
}

// UnblockUsers unblocks previously blocked users.
// Mirrors pywa's WhatsApp.unblock_users.
//
// Parameters:
//   - users:   slice of WA IDs.
//   - phoneID: optional phone ID override.
//
// Returns:
//   - *UsersUnblockedResult.
//   - error.
func (wa *WhatsApp) UnblockUsers(users []string, phoneID ...string) (*UsersUnblockedResult, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid := ""
	if len(phoneID) > 0 {
		pid = phoneID[0]
	}
	pid, err := wa.resolveSender(pid)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.unblockUsers(pid, users)
	if err != nil {
		return nil, err
	}
	result := &UsersUnblockedResult{}
	for _, u := range toAnySlice(res["removed_users"]) {
		um := toMap(u)
		result.RemovedUsers = append(result.RemovedUsers, UnblockedUser{WAID: toString(um["wa_id"])})
	}
	return result, nil
}

// ── Calling ───────────────────────────────────────────────────────────────────

// InitiateCallOptions holds optional parameters for InitiateCall.
type InitiateCallOptions struct {
	Tracker string
	PhoneID string
}

// InitiateCall initiates an outbound voice call.
// Mirrors pywa's WhatsApp.initiate_call.
//
// Parameters:
//   - to:  callee WA ID.
//   - sdp: SessionDescription for the WebRTC connection.
//   - opts: optional InitiateCallOptions.
//
// Returns:
//   - *InitiatedCall.
//   - error.
func (wa *WhatsApp) InitiateCall(to string, sdp SessionDescription, opts ...InitiateCallOptions) (*InitiatedCall, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := InitiateCallOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	pid, err := wa.resolveSender(o.PhoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.initiateCall(pid, to, sdp.toDict(), o.Tracker)
	if err != nil {
		return nil, err
	}
	sm := extractSentMessage(res, pid, to)
	callID := toString(res["call_id"])
	return &InitiatedCall{SentMessage: *sm, CallID: callID}, nil
}

// ── Phone number management ───────────────────────────────────────────────────

// RegisterPhoneNumber registers a phone number for WhatsApp Business Cloud API.
// Mirrors pywa's WhatsApp.register_phone_number.
//
// Parameters:
//   - pin:                   6-digit 2-step verification PIN.
//   - dataLocalizationRegion: ISO-3166 country code for data-at-rest locality (e.g. "IN").
//   - phoneID:               optional phone ID override.
//
// Returns:
//   - error.
func (wa *WhatsApp) RegisterPhoneNumber(pin, dataLocalizationRegion, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	return wa.api.registerPhoneNumber(pid, pin, dataLocalizationRegion)
}

// DeregisterPhoneNumber deregisters a phone number.
// Mirrors pywa's WhatsApp.deregister_phone_number.
//
// Parameters:
//   - phoneID: optional phone ID override.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeregisterPhoneNumber(phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	return wa.api.deregisterPhoneNumber(pid)
}

// ── Display name ──────────────────────────────────────────────────────────────

// UpdateDisplayName updates the business display name.
// Mirrors pywa's WhatsApp.update_display_name.
//
// Parameters:
//   - newDisplayName: the new name string.
//   - phoneID:        optional phone ID override.
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateDisplayName(newDisplayName, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	return wa.api.updateDisplayName(pid, newDisplayName)
}

// ── Conversational automation ─────────────────────────────────────────────────

// UpdateConversationalAutomationOptions holds options for UpdateConversationalAutomation.
type UpdateConversationalAutomationOptions struct {
	IceBreakers []string
	Commands    []Command
	PhoneID     string
}

// UpdateConversationalAutomation configures ice-breakers and slash-commands.
// Mirrors pywa's WhatsApp.update_conversational_automation.
//
// Parameters:
//   - enableChatOpened: whether to receive ChatOpened events.
//   - opts:             optional UpdateConversationalAutomationOptions.
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateConversationalAutomation(enableChatOpened bool, opts ...UpdateConversationalAutomationOptions) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	o := UpdateConversationalAutomationOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	pid, err := wa.resolveSender(o.PhoneID)
	if err != nil {
		return err
	}
	var commandsJSON string
	if len(o.Commands) > 0 {
		dicts := make([]map[string]any, len(o.Commands))
		for i, c := range o.Commands {
			dicts[i] = c.toDict()
		}
		b, _ := json.Marshal(dicts)
		commandsJSON = string(b)
	}
	return wa.api.updateConversationalAutomation(pid, enableChatOpened, o.IceBreakers, commandsJSON)
}

// ── Commerce settings ─────────────────────────────────────────────────────────

// GetCommerceSettings fetches catalog/cart settings.
// Mirrors pywa's WhatsApp.get_commerce_settings.
//
// Parameters:
//   - phoneID: optional override.
//
// Returns:
//   - *CommerceSettings.
//   - error.
func (wa *WhatsApp) GetCommerceSettings(phoneID string) (*CommerceSettings, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.getCommerceSettings(pid, "is_cart_enabled,is_catalog_visible")
	if err != nil {
		return nil, err
	}
	data := toAnySlice(res["data"])
	if len(data) == 0 {
		return &CommerceSettings{}, nil
	}
	dm := toMap(data[0])
	return &CommerceSettings{
		IsCatalogVisible: toBool(dm["is_catalog_visible"]),
		IsCartEnabled:    toBool(dm["is_cart_enabled"]),
	}, nil
}

// UpdateCommerceSettings updates catalog/cart settings.
// Mirrors pywa's WhatsApp.update_commerce_settings.
//
// Parameters:
//   - isCatalogVisible: optional; nil means "don't change".
//   - isCartEnabled:    optional; nil means "don't change".
//   - phoneID:          optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateCommerceSettings(isCatalogVisible, isCartEnabled *bool, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	if isCatalogVisible == nil && isCartEnabled == nil {
		return fmt.Errorf("gowa: at least one of isCatalogVisible or isCartEnabled must be provided")
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	data := map[string]any{}
	if isCatalogVisible != nil {
		data["is_catalog_visible"] = *isCatalogVisible
	}
	if isCartEnabled != nil {
		data["is_cart_enabled"] = *isCartEnabled
	}
	return wa.api.updateCommerceSettings(pid, data)
}

// ── App access token ──────────────────────────────────────────────────────────

// GetAppAccessToken fetches a client_credentials token for a Meta app.
// Mirrors pywa's WhatsApp.get_app_access_token.
//
// Parameters:
//   - appID:     the numeric app ID.
//   - appSecret: the app secret.
//
// Returns:
//   - token string.
//   - error.
func (wa *WhatsApp) GetAppAccessToken(appID, appSecret string) (string, error) {
	if err := wa.requireAPI(); err != nil {
		return "", err
	}
	return wa.api.getAppAccessToken(appID, appSecret)
}

// SetAppCallbackURL registers the webhook callback URL for a Meta app.
// Mirrors pywa's WhatsApp.set_app_callback_url.
//
// Parameters:
//   - appID:          Meta app ID (numeric).
//   - appAccessToken: token from GetAppAccessToken.
//   - callbackURL:    public HTTPS endpoint.
//   - verifyToken:    challenge verify string.
//   - fields:         webhook fields to subscribe to.
//
// Returns:
//   - error.
func (wa *WhatsApp) SetAppCallbackURL(appID int, appAccessToken, callbackURL, verifyToken string, fields []string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	return wa.api.setAppCallbackURL(appID, appAccessToken, callbackURL, verifyToken, fields)
}

// ── Interactive builder helpers ───────────────────────────────────────────────

// buildInteractivePayload builds an interactive message payload for text-body messages.
func buildInteractivePayload(body, header, footer string, buttons any) map[string]any {
	m := map[string]any{
		"body": map[string]any{"text": body},
	}
	if header != "" {
		m["header"] = map[string]any{"type": "text", "text": header}
	}
	if footer != "" {
		m["footer"] = map[string]any{"text": footer}
	}
	applyButtons(m, buttons)
	return m
}

// buildInteractivePayloadWithHeader builds an interactive payload with a media header.
func buildInteractivePayloadWithHeader(body, footer string, buttons any, header map[string]any) map[string]any {
	m := map[string]any{
		"body":   map[string]any{"text": body},
		"header": header,
	}
	if footer != "" {
		m["footer"] = map[string]any{"text": footer}
	}
	applyButtons(m, buttons)
	return m
}

// applyButtons adds the type and action fields for an interactive payload.
func applyButtons(m map[string]any, buttons any) {
	switch b := buttons.(type) {
	case []Button:
		btnMaps := make([]map[string]any, len(b))
		for i, btn := range b {
			btnMaps[i] = map[string]any{
				"type":  "reply",
				"reply": map[string]any{"id": btn.ID, "title": btn.Title},
			}
		}
		m["type"] = "button"
		m["action"] = map[string]any{"buttons": btnMaps}
	case *SectionList:
		m["type"] = "list"
		m["action"] = b.toDict()
	case *URLButton:
		m["type"] = "cta_url"
		m["action"] = map[string]any{
			"name":       "cta_url",
			"parameters": map[string]any{"display_text": b.Title, "url": b.URL},
		}
	case *FlowButton:
		m["type"] = "flow"
		m["action"] = map[string]any{
			"name": "flow",
			"parameters": map[string]any{
				"flow_message_version": "3",
				"flow_token":           b.FlowToken,
				"flow_id":              b.FlowID,
				"flow_cta":             b.Text,
				"flow_action":          "navigate",
				"flow_action_payload": map[string]any{
					"screen": b.NavigateTo,
					"data":   b.FlowData,
				},
			},
		}
	}
}

// ── MIME type helpers ─────────────────────────────────────────────────────────

// mimeTypeFromPath infers a MIME type from a file extension.
// Falls back to "application/octet-stream".
func mimeTypeFromPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg":
		return "audio/ogg"
	case ".opus":
		return "audio/opus"
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

// extensionFromMimeType returns a common file extension for the given MIME type.
func extensionFromMimeType(mimeType string) (string, bool) {
	// strip parameters like "; codecs=opus"
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	switch mimeType {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	case "video/mp4":
		return ".mp4", true
	case "audio/mpeg":
		return ".mp3", true
	case "audio/ogg":
		return ".ogg", true
	case "application/pdf":
		return ".pdf", true
	default:
		return ".bin", false
	}
}
