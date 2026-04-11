package gowa

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// ────────────────────────────────────────────────────────────────────────────────
// Webhook server
// Mirrors pywa's server.py — the layer that registers HTTP routes, validates
// signatures, parses payloads, and dispatches to the registered handlers.
//
// Unlike pywa (which tightly couples to Flask/FastAPI), gowa exposes:
//
//   1. Handler() http.Handler — attach to any net/http mux or framework
//   2. ListenAndServe(addr)   — stand-alone HTTP server (convenience wrapper)
//   3. HandleWebhookUpdate    — manual injection for custom HTTP stacks
// ────────────────────────────────────────────────────────────────────────────────

const (
	// hubModeParam is the query parameter for verification challenge mode.
	hubModeParam = "hub.mode"
	// hubTokenParam is the query parameter for the verify token.
	hubTokenParam = "hub.verify_token"
	// hubChallengeParam is the query parameter for the challenge string.
	hubChallengeParam = "hub.challenge"
	// signatureHeader is the header carrying the HMAC-SHA256 signature.
	signatureHeader = "X-Hub-Signature-256"
)

// Handler returns an http.Handler for the webhook endpoint.
// Mount this on your own HTTP server at the configured endpoint path.
//
// Example — stdlib:
//
//	mux := http.NewServeMux()
//	mux.Handle(wa.WebhookEndpoint(), wa.Handler())
//	http.ListenAndServe(":8080", mux)
//
// Example — Gin:
//
//	r := gin.Default()
//	r.Any(wa.WebhookEndpoint(), gin.WrapH(wa.Handler()))
//
// Returns:
//   - http.Handler that handles GET (challenge) and POST (update) requests.
func (wa *WhatsApp) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			wa.handleChallenge(w, r)
		case http.MethodPost:
			wa.handleUpdate(w, r)
		default:
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		}
	})
}

// WebhookEndpoint returns the configured webhook path (e.g. "/webhook").
// Use this when registering the handler on an external mux.
//
// Returns:
//   - the webhook path string.
func (wa *WhatsApp) WebhookEndpoint() string {
	return wa.webhookEndpoint
}

// ListenAndServe starts a stand-alone HTTP server on the given address.
// The webhook is registered at wa.WebhookEndpoint().
//
// Parameters:
//   - addr: TCP address to listen on (e.g. ":8080").
//
// Returns:
//   - error if the server fails to start or exits unexpectedly.
func (wa *WhatsApp) ListenAndServe(addr string) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle(wa.webhookEndpoint, wa.Handler())
	log.Printf("[gowa] Webhook server listening on %s%s", addr, wa.webhookEndpoint)
	return http.ListenAndServe(addr, mux)
}

// HandleWebhookUpdate processes a raw webhook payload manually.
// Use this when integrating with a custom HTTP framework that does not use
// net/http.Handler (e.g. Gin's c.Request.Body has already been read).
//
// Parameters:
//   - body:      raw request bytes.
//   - signature: value of the X-Hub-Signature-256 header (empty skips validation).
//
// Returns:
//   - HTTP status code (200 on success, 401/400 on error).
//   - response body string.
func (wa *WhatsApp) HandleWebhookUpdate(body []byte, signature string) (int, string) {
	if wa.appSecret != "" && signature != "" {
		if !validateSignature(wa.appSecret, body, signature) {
			return http.StatusUnauthorized, "invalid signature"
		}
	}
	return wa.processRawUpdate(body)
}

// ── Internal HTTP handlers ────────────────────────────────────────────────────

// handleChallenge responds to the Meta webhook verification GET request.
//
// Parameters:
//   - w: http.ResponseWriter.
//   - r: *http.Request with hub.* query parameters.
func (wa *WhatsApp) handleChallenge(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	mode := q.Get(hubModeParam)
	vt := q.Get(hubTokenParam)
	ch := q.Get(hubChallengeParam)

	if mode != "subscribe" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if vt != wa.verifyToken {
		log.Printf("[gowa] webhook verification failed: got verify_token=%q", vt)
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	log.Printf("[gowa] webhook at %s verified successfully", wa.webhookEndpoint)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(ch))
}

// handleUpdate is the POST handler for incoming webhook events.
//
// Parameters:
//   - w: http.ResponseWriter.
//   - r: *http.Request containing the update payload.
func (wa *WhatsApp) handleUpdate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	sig := r.Header.Get(signatureHeader)
	if wa.appSecret != "" {
		if sig == "" {
			http.Error(w, "missing signature", http.StatusUnauthorized)
			return
		}
		if !validateSignature(wa.appSecret, body, sig) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	status, msg := wa.processRawUpdate(body)
	w.WriteHeader(status)
	if msg != "" {
		_, _ = w.Write([]byte(msg))
	}
}

// processRawUpdate decodes the JSON payload and dispatches to handlers.
//
// Parameters:
//   - body: raw JSON bytes from the webhook.
//
// Returns:
//   - HTTP status code.
//   - response string.
func (wa *WhatsApp) processRawUpdate(body []byte) (int, string) {
	var raw RawUpdate
	if err := json.Unmarshal(body, &raw); err != nil {
		log.Printf("[gowa] failed to decode update: %v", err)
		return http.StatusBadRequest, "invalid JSON"
	}

	// Dispatch raw handlers first (they see everything, no filtering)
	wa.hdlrs.raw.dispatch(wa, raw)

	// Classify and dispatch to typed handlers
	wa.dispatchUpdate(raw)

	return http.StatusOK, ""
}

// dispatchUpdate routes the decoded raw update to the correct typed handler list.
// Mirrors pywa's Server._call_handlers.
//
// Parameters:
//   - raw: the fully decoded top-level webhook JSON object.
func (wa *WhatsApp) dispatchUpdate(raw RawUpdate) {
	entry, ok := raw["entry"].([]any)
	if !ok || len(entry) == 0 {
		return
	}

	for _, e := range entry {
		eMap, ok := e.(map[string]any)
		if !ok {
			continue
		}

		changes, ok := eMap["changes"].([]any)
		if !ok {
			continue
		}

		for _, ch := range changes {
			chMap, ok := ch.(map[string]any)
			if !ok {
				continue
			}
			field, _ := chMap["field"].(string)
			value, _ := chMap["value"].(map[string]any)
			if value == nil {
				continue
			}

			// Filter by phone_id if configured
			if wa.phoneID != "" {
				meta, _ := value["metadata"].(map[string]any)
				if meta != nil {
					if pid, _ := meta["phone_number_id"].(string); pid != wa.phoneID {
						continue
					}
				}
			}

			switch field {
			case "messages":
				wa.dispatchMessagesField(value)
			case "message_template_status_update":
				wa.dispatchTemplateStatusUpdate(value)
			case "message_template_quality_update":
				wa.dispatchTemplateQualityUpdate(value)
			case "marketing_messages":
				wa.dispatchMarketingPreferences(value)
			}
		}
	}
}

// dispatchMessagesField handles the "messages" webhook field, which contains
// messages, statuses, contacts, and interactive payloads.
//
// Parameters:
//   - value: the "value" object from the change entry.
func (wa *WhatsApp) dispatchMessagesField(value map[string]any) {
	metadata := parseMetadata(value)

	// Incoming user messages
	if msgs, ok := value["messages"].([]any); ok {
		for _, m := range msgs {
			mMap, ok := m.(map[string]any)
			if !ok {
				continue
			}
			wa.dispatchMessage(metadata, mMap, value)
		}
	}

	// Message status updates
	if statuses, ok := value["statuses"].([]any); ok {
		for _, s := range statuses {
			sMap, ok := s.(map[string]any)
			if !ok {
				continue
			}
			wa.dispatchStatus(metadata, sMap)
		}
	}
}

// dispatchMessage routes a single message object to the appropriate handler(s).
//
// Parameters:
//   - meta: parsed Metadata for this message batch.
//   - msg:  raw message map from the webhook.
//   - value: parent value map (may contain contacts[]).
func (wa *WhatsApp) dispatchMessage(meta Metadata, msg map[string]any, value map[string]any) {
	msgType := MessageType(toString(msg["type"]))
	from := parseUser(value, msg)
	ts := toTime(msg["timestamp"])
	id := toString(msg["id"])

	base := BaseUserUpdate{
		ID:        id,
		Metadata:  meta,
		From:      from,
		Timestamp: ts,
		client:    wa,
	}

	switch msgType {
	case MessageTypeButton:
		// Quick-reply button tap (legacy format)
		btnData := toMap(msg["button"])
		cb := &CallbackButton{
			BaseUserUpdate: base,
			Title:          toString(btnData["text"]),
			Data:           toString(btnData["payload"]),
		}
		wa.hdlrs.callbackButton.dispatch(wa, cb)

	case MessageTypeInteractive:
		interactive := toMap(msg["interactive"])
		iType := toString(interactive["type"])

		switch iType {
		case "button_reply":
			br := toMap(interactive["button_reply"])
			cb := &CallbackButton{
				BaseUserUpdate: base,
				Title:          toString(br["title"]),
				Data:           toString(br["id"]),
			}
			wa.hdlrs.callbackButton.dispatch(wa, cb)

		case "list_reply":
			lr := toMap(interactive["list_reply"])
			cs := &CallbackSelection{
				BaseUserUpdate: base,
				Title:          toString(lr["title"]),
				Data:           toString(lr["id"]),
				Description:    toString(lr["description"]),
			}
			wa.hdlrs.callbackSelect.dispatch(wa, cs)

		case "nfm_reply":
			// Flow completion
			nfm := toMap(interactive["nfm_reply"])
			body := toMap(nfm["response_json"])
			fc := &FlowCompletion{
				BaseUserUpdate: base,
				FlowToken:      toString(body["flow_token"]),
				Response:       body,
			}
			wa.hdlrs.flowCompletion.dispatch(wa, fc)

		case "call_permission_reply":
			cpu := &CallPermissionUpdate{
				BaseUserUpdate: base,
				Response:       toString(interactive["call_permission_reply"]),
			}
			wa.hdlrs.callPermission.dispatch(wa, cpu)
		}

	case MessageTypeRequestWelcome:
		co := &ChatOpened{
			Metadata:  meta,
			Timestamp: ts,
			From:      from,
		}
		wa.hdlrs.chatOpened.dispatch(wa, co)

	case MessageTypeSystem:
		sys := toMap(msg["system"])
		sysType := toString(sys["type"])
		switch sysType {
		case "user_changed_number", "customer_changed_number":
			pnc := &PhoneNumberChange{
				Metadata:  meta,
				Timestamp: ts,
				OldWAID:   toString(sys["customer"]),
				NewWAID:   toString(sys["new_wa_id"]),
			}
			wa.hdlrs.phoneNumChange.dispatch(wa, pnc)
		case "customer_identity_changed":
			ic := &IdentityChange{
				Metadata:         meta,
				Timestamp:        ts,
				From:             from,
				CreatedTimestamp: toTime(sys["identity_key_creation_timestamp"]),
				Hash:             toString(sys["acknowledged_country"]),
			}
			wa.hdlrs.identityChange.dispatch(wa, ic)
		}

	default:
		// Standard message types (text, image, video, etc.)
		m := parseMessage(base, msgType, msg)
		// Give blocking listeners first chance to consume the message.
		if wa.notifyListeners(m) {
			return
		}
		wa.hdlrs.message.dispatch(wa, m)
	}
}

// dispatchStatus routes a message status update to the messageStatus handler list.
//
// Parameters:
//   - meta:   Metadata for the batch.
//   - status: raw status map from the webhook.
func (wa *WhatsApp) dispatchStatus(meta Metadata, status map[string]any) {
	var apiErr *WhatsAppError
	if errMap := toMap(status["errors"]); errMap != nil {
		apiErr = whatsAppErrorFromMap(errMap, nil)
	} else if errs, ok := status["errors"].([]any); ok && len(errs) > 0 {
		if em, ok := errs[0].(map[string]any); ok {
			apiErr = whatsAppErrorFromMap(em, nil)
		}
	}

	ms := &MessageStatus{
		ID:        toString(status["id"]),
		Metadata:  meta,
		Status:    MessageStatusType(toString(status["status"])),
		Timestamp: toTime(status["timestamp"]),
		From: User{
			WAID:   toString(status["recipient_id"]),
			client: wa,
		},
		TrackerID: toString(status["biz_opaque_callback_data"]),
		Error:     apiErr,
	}
	wa.hdlrs.messageStatus.dispatch(wa, ms)
}

// dispatchTemplateStatusUpdate routes template status changes.
//
// Parameters:
//   - value: raw change value map.
func (wa *WhatsApp) dispatchTemplateStatusUpdate(value map[string]any) {
	u := &TemplateStatusUpdate{
		TemplateID:   toString(value["message_template_id"]),
		TemplateName: toString(value["message_template_name"]),
		Status:       TemplateStatus(toString(value["event"])),
		Reason:       toString(value["reason"]),
	}
	wa.hdlrs.tmplStatus.dispatch(wa, u)
}

// dispatchTemplateQualityUpdate routes template quality score changes.
//
// Parameters:
//   - value: raw change value map.
func (wa *WhatsApp) dispatchTemplateQualityUpdate(value map[string]any) {
	u := &TemplateQualityUpdate{
		TemplateID:   toString(value["message_template_id"]),
		TemplateName: toString(value["message_template_name"]),
		QualityScore: toString(value["quality_score"]),
	}
	wa.hdlrs.tmplQuality.dispatch(wa, u)
}

// dispatchMarketingPreferences routes user marketing preference updates.
//
// Parameters:
//   - value: raw change value map.
func (wa *WhatsApp) dispatchMarketingPreferences(value map[string]any) {
	meta := parseMetadata(value)
	contacts, _ := value["contacts"].([]any)
	for _, c := range contacts {
		cMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		u := &UserMarketingPreferences{
			Metadata:  meta,
			Timestamp: toTime(value["timestamp"]),
			From:      User{WAID: toString(cMap["wa_id"]), client: wa},
			OptIn:     toString(value["marketing_opt_in_opted_in"]) == "true",
		}
		wa.hdlrs.userMktgPrefs.dispatch(wa, u)
	}
}

// ── Signature validation ──────────────────────────────────────────────────────

// validateSignature verifies the HMAC-SHA256 signature on an incoming webhook payload.
// Mirrors pywa's utils.webhook_updates_validator.
//
// Parameters:
//   - appSecret: the Meta app secret.
//   - body:      raw request bytes.
//   - sigHeader: value of the X-Hub-Signature-256 header (format "sha256=HEXHASH").
//
// Returns:
//   - true if the signature matches, false otherwise.
func validateSignature(appSecret string, body []byte, sigHeader string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(sigHeader, prefix) {
		return false
	}
	sig := sigHeader[len(prefix):]
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

// ── Parse helpers ─────────────────────────────────────────────────────────────

// parseMetadata extracts the Metadata struct from a webhook value map.
func parseMetadata(value map[string]any) Metadata {
	meta := toMap(value["metadata"])
	return Metadata{
		DisplayPhoneNumber: toString(meta["display_phone_number"]),
		PhoneNumberID:      toString(meta["phone_number_id"]),
	}
}

// parseUser extracts the sender User from a webhook value + message map.
func parseUser(value, msg map[string]any) User {
	// The "contacts" array at the value level holds profile names
	var name string
	if contacts, ok := value["contacts"].([]any); ok && len(contacts) > 0 {
		if cm, ok := contacts[0].(map[string]any); ok {
			if profile, ok := cm["profile"].(map[string]any); ok {
				name = toString(profile["name"])
			}
		}
	}
	return User{
		WAID: toString(msg["from"]),
		Name: name,
	}
}

// parseMessage constructs a *Message from the raw webhook map.
func parseMessage(base BaseUserUpdate, msgType MessageType, msg map[string]any) *Message {
	m := &Message{
		BaseUserUpdate: base,
		Type:           msgType,
	}

	// Forwarding context
	if ctx, ok := msg["context"].(map[string]any); ok {
		m.Forwarded = toBool(ctx["forwarded"])
		m.ForwardedManyTimes = toBool(ctx["frequently_forwarded"])
		if replyID := toString(ctx["id"]); replyID != "" {
			m.ReplyToMessage = &ReplyToMessage{
				MessageID: replyID,
				FromWAID:  toString(ctx["from"]),
			}
		}
	}

	// Referral (click-to-WA ads)
	if ref := toMap(msg["referral"]); ref != nil {
		m.Referral = &Referral{
			SourceURL:  toString(ref["source_url"]),
			SourceID:   toString(ref["source_id"]),
			SourceType: toString(ref["source_type"]),
			Headline:   toString(ref["headline"]),
			Body:       toString(ref["body"]),
			MediaType:  toString(ref["media_type"]),
			ImageURL:   toString(ref["image_url"]),
			VideoURL:   toString(ref["video_url"]),
			CTWAClid:   toString(ref["ctwa_clid"]),
		}
	}

	// Errors embedded in the message
	if errs, ok := msg["errors"].([]any); ok && len(errs) > 0 {
		if em, ok := errs[0].(map[string]any); ok {
			m.Error = whatsAppErrorFromMap(em, nil)
		}
	}

	switch msgType {
	case MessageTypeText:
		textObj := toMap(msg["text"])
		body := toString(textObj["body"])
		m.Text = &body

	case MessageTypeImage:
		img := toMap(msg["image"])
		m.Image = &Image{
			MediaBase: parseMediaBase(img, base.client),
			Caption:   toString(img["caption"]),
		}

	case MessageTypeVideo:
		vid := toMap(msg["video"])
		m.Video = &Video{
			MediaBase: parseMediaBase(vid, base.client),
			Caption:   toString(vid["caption"]),
		}

	case MessageTypeAudio:
		aud := toMap(msg["audio"])
		m.Audio = &Audio{
			MediaBase: parseMediaBase(aud, base.client),
			Voice:     toBool(aud["voice"]),
		}

	case MessageTypeDocument:
		doc := toMap(msg["document"])
		m.Document = &Document{
			MediaBase: parseMediaBase(doc, base.client),
			Caption:   toString(doc["caption"]),
			Filename:  toString(doc["filename"]),
		}

	case MessageTypeSticker:
		stk := toMap(msg["sticker"])
		m.Sticker = &Sticker{
			MediaBase: parseMediaBase(stk, base.client),
			Animated:  toBool(stk["animated"]),
		}

	case MessageTypeReaction:
		react := toMap(msg["reaction"])
		m.Reaction = &Reaction{
			MessageID: toString(react["message_id"]),
			Emoji:     toString(react["emoji"]),
		}

	case MessageTypeLocation:
		loc := toMap(msg["location"])
		m.Location = &Location{
			Latitude:  toFloat64(loc["latitude"]),
			Longitude: toFloat64(loc["longitude"]),
			Name:      toString(loc["name"]),
			Address:   toString(loc["address"]),
			URL:       toString(loc["url"]),
		}

	case MessageTypeContacts:
		if arr, ok := msg["contacts"].([]any); ok {
			for _, ci := range arr {
				cm, ok := ci.(map[string]any)
				if !ok {
					continue
				}
				m.Contacts = append(m.Contacts, parseContact(cm))
			}
		}

	case MessageTypeOrder:
		ord := toMap(msg["order"])
		items := parseOrderItems(toAnySlice(ord["product_items"]))
		m.Order = &Order{
			CatalogID:    toString(ord["catalog_id"]),
			Text:         toString(ord["text"]),
			ProductItems: items,
		}

	default:
		unsuppType := string(msgType)
		if unsuppType == "" {
			unsuppType = toString(msg["type"])
		}
		m.Unsupported = &Unsupported{MessageType: unsuppType}
		m.Type = MessageTypeUnsupported
	}

	return m
}

// parseMediaBase extracts common media fields.
func parseMediaBase(m map[string]any, client *WhatsApp) MediaBase {
	return MediaBase{
		ID:       toString(m["id"]),
		SHA256:   toString(m["sha256"]),
		MimeType: toString(m["mime_type"]),
		client:   client,
	}
}

// parseContact converts a raw contact map to a Contact struct.
func parseContact(cm map[string]any) Contact {
	c := Contact{}
	if n := toMap(cm["name"]); n != nil {
		c.Name = ContactName{
			FormattedName: toString(n["formatted_name"]),
			FirstName:     toString(n["first_name"]),
			LastName:      toString(n["last_name"]),
			MiddleName:    toString(n["middle_name"]),
			Suffix:        toString(n["suffix"]),
			Prefix:        toString(n["prefix"]),
		}
	}
	for _, p := range toAnySlice(cm["phones"]) {
		pm := toMap(p)
		if pm == nil {
			continue
		}
		c.Phones = append(c.Phones, ContactPhone{
			Phone: toString(pm["phone"]),
			WAID:  toString(pm["wa_id"]),
			Type:  toString(pm["type"]),
		})
	}
	for _, e := range toAnySlice(cm["emails"]) {
		em := toMap(e)
		if em == nil {
			continue
		}
		c.Emails = append(c.Emails, ContactEmail{Email: toString(em["email"]), Type: toString(em["type"])})
	}
	for _, u := range toAnySlice(cm["urls"]) {
		um := toMap(u)
		if um == nil {
			continue
		}
		c.URLs = append(c.URLs, ContactURL{URL: toString(um["url"]), Type: toString(um["type"])})
	}
	return c
}

// parseOrderItems converts a []any of product_items into []OrderItem.
func parseOrderItems(arr []any) []OrderItem {
	items := make([]OrderItem, 0, len(arr))
	for _, raw := range arr {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, OrderItem{
			ProductRetailerID: toString(m["product_retailer_id"]),
			Quantity:          int(toFloat64(m["quantity"])),
			ItemPrice:         toFloat64(m["item_price"]),
			Currency:          toString(m["currency"]),
		})
	}
	return items
}

// toMap is a nil-safe type assertion to map[string]any.
func toMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// toAnySlice is a nil-safe type assertion to []any.
func toAnySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// requireWebhook returns ErrNoWebhook when the client was created without server config.
func (wa *WhatsApp) requireWebhook() error {
	if wa.verifyToken == "" {
		return fmt.Errorf("%w: configure verify_token to receive webhook updates", ErrNoWebhook)
	}
	return nil
}
