package gowa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────────
// GraphAPI — internal HTTP client for the Meta Graph API.
// Mirrors pywa's GraphAPI class (pywa/api.py).
// All requests are made with the stdlib net/http package.
// ────────────────────────────────────────────────────────────────────────────────

const (
	// defaultAPIVersion is the Graph API version used when none is specified.
	// Mirrors pywa's utils.Version.GRAPH_API.
	defaultAPIVersion = "22.0"

	// graphAPIBase is the root URL of the Meta Graph API.
	graphAPIBase = "https://graph.facebook.com"
)

// graphAPI is the low-level HTTP client for the WhatsApp Cloud API.
// One instance is held by each WhatsApp client.
type graphAPI struct {
	httpClient *http.Client
	token      string
	baseURL    string // e.g. "https://graph.facebook.com/v22.0"
	version    string
}

// newGraphAPI constructs a graphAPI.
//
// Parameters:
//   - token:      Bearer access token.
//   - httpClient: optional *http.Client (nil → default client with 30 s timeout).
//   - version:    Graph API version string (e.g. "22.0"); empty → defaultAPIVersion.
//
// Returns:
//   - *graphAPI ready for use.
func newGraphAPI(token string, httpClient *http.Client, version string) *graphAPI {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	if version == "" {
		version = defaultAPIVersion
	}
	return &graphAPI{
		httpClient: httpClient,
		token:      token,
		baseURL:    fmt.Sprintf("%s/v%s", graphAPIBase, version),
		version:    version,
	}
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

// request is the single internal method for all API calls.
// It handles auth headers, JSON body/response, and error mapping.
//
// Parameters:
//   - method:       HTTP verb (GET, POST, DELETE).
//   - endpoint:     path (may start with "/" or be a full URL).
//   - params:       URL query parameters (may be nil).
//   - body:         JSON-serialisable request body (may be nil for GET).
//   - multipartFn:  optional function to write a multipart body; when non-nil,
//     body is ignored and Content-Type is set to multipart/form-data.
//
// Returns:
//   - map[string]any: the decoded JSON response.
//   - error: *WhatsAppError on API error, or stdlib error on network failure.
func (g *graphAPI) request(
	method, endpoint string,
	params map[string]string,
	body any,
	multipartFn func(*multipart.Writer) error,
) (map[string]any, error) {

	// Build full URL
	var rawURL string
	if strings.HasPrefix(endpoint, "https://") || strings.HasPrefix(endpoint, "http://") {
		rawURL = endpoint
	} else {
		rawURL = g.baseURL + endpoint
	}

	var reqBody io.Reader
	var contentType string

	if multipartFn != nil {
		// Multipart form upload (media)
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		if err := multipartFn(mw); err != nil {
			return nil, fmt.Errorf("building multipart body: %w", err)
		}
		if err := mw.Close(); err != nil {
			return nil, err
		}
		reqBody = &buf
		contentType = mw.FormDataContentType()
	} else if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshalling request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
		contentType = "application/json"
	}

	req, err := http.NewRequest(method, rawURL, reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating HTTP request: %w", err)
	}

	// Auth
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("User-Agent", "gowa/1.0")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	// Query params
	if len(params) > 0 {
		q := req.URL.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		req.URL.RawQuery = q.Encode()
	}

	log.Printf("[gowa] %s %s", method, req.URL.String())

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP %s %s: %w", method, rawURL, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(respBytes, &result); err != nil {
		// Some endpoints return non-JSON on success
		if resp.StatusCode < 400 {
			return map[string]any{"raw": string(respBytes)}, nil
		}
		return nil, fmt.Errorf("decoding error response: %w (body: %s)", err, string(respBytes))
	}

	if resp.StatusCode >= 400 {
		errMap, _ := result["error"].(map[string]any)
		if errMap == nil {
			errMap = map[string]any{"code": resp.StatusCode, "message": string(respBytes)}
		}
		return nil, whatsAppErrorFromMap(errMap, resp)
	}

	return result, nil
}

// get is a convenience wrapper for GET requests.
func (g *graphAPI) get(endpoint string, params map[string]string) (map[string]any, error) {
	return g.request(http.MethodGet, endpoint, params, nil, nil)
}

// post is a convenience wrapper for POST requests with a JSON body.
func (g *graphAPI) post(endpoint string, body any) (map[string]any, error) {
	return g.request(http.MethodPost, endpoint, nil, body, nil)
}

// postForm is a convenience wrapper for POST requests with URL-form params.
func (g *graphAPI) postForm(endpoint string, params map[string]string) (map[string]any, error) {
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	req, err := http.NewRequest(http.MethodPost, g.baseURL+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		errMap, _ := result["error"].(map[string]any)
		return nil, whatsAppErrorFromMap(errMap, resp)
	}
	return result, nil
}

// delete is a convenience wrapper for DELETE requests.
func (g *graphAPI) delete(endpoint string, params map[string]string) (map[string]any, error) {
	return g.request(http.MethodDelete, endpoint, params, nil, nil)
}

// ── Message sending ───────────────────────────────────────────────────────────

// sendMessagePayload is the JSON body for the /messages endpoint.
// Fields are omitempty so zero-values are excluded automatically.
type sendMessagePayload struct {
	MessagingProduct         string           `json:"messaging_product"`
	RecipientType            string           `json:"recipient_type"`
	To                       string           `json:"to"`
	Type                     string           `json:"type"`
	Text                     map[string]any   `json:"text,omitempty"`
	Image                    map[string]any   `json:"image,omitempty"`
	Video                    map[string]any   `json:"video,omitempty"`
	Audio                    map[string]any   `json:"audio,omitempty"`
	Document                 map[string]any   `json:"document,omitempty"`
	Sticker                  map[string]any   `json:"sticker,omitempty"`
	Reaction                 map[string]any   `json:"reaction,omitempty"`
	Location                 map[string]any   `json:"location,omitempty"`
	Contacts                 []map[string]any `json:"contacts,omitempty"`
	Interactive              map[string]any   `json:"interactive,omitempty"`
	Template                 map[string]any   `json:"template,omitempty"`
	Context                  map[string]any   `json:"context,omitempty"`
	BizOpaqueCallbackData    string           `json:"biz_opaque_callback_data,omitempty"`
	RecipientIdentityKeyHash string           `json:"recipient_identity_key_hash,omitempty"`
}

// sendMessage dispatches a message to the /PHONE_ID/messages endpoint.
//
// Parameters:
//   - phoneID:             Sender phone number ID.
//   - payload:             Fully populated sendMessagePayload.
//
// Returns:
//   - map[string]any: API response {"messages":[{"id":"wamid..."}],...}.
//   - error: *WhatsAppError on API failure, stdlib error otherwise.
func (g *graphAPI) sendMessage(phoneID string, payload sendMessagePayload) (map[string]any, error) {
	payload.MessagingProduct = "whatsapp"
	payload.RecipientType = "individual"
	return g.post("/"+phoneID+"/messages", payload)
}

// ── Media upload ──────────────────────────────────────────────────────────────

// uploadMedia uploads raw media bytes to WhatsApp servers.
//
// Parameters:
//   - phoneID:   Phone number ID to associate the upload with.
//   - fileData:  Raw file content.
//   - mimeType:  MIME type (e.g. "image/jpeg").
//   - filename:  Filename hint.
//
// Returns:
//   - mediaID string from the API.
//   - error if the upload fails.
func (g *graphAPI) uploadMedia(phoneID string, fileData []byte, mimeType, filename string) (string, error) {
	result, err := g.request(
		http.MethodPost,
		"/"+phoneID+"/media",
		nil, nil,
		func(mw *multipart.Writer) error {
			if err := mw.WriteField("messaging_product", "whatsapp"); err != nil {
				return err
			}
			if err := mw.WriteField("type", mimeType); err != nil {
				return err
			}

			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
			h.Set("Content-Type", mimeType)

			part, err := mw.CreateFormFile("file", filename)
			if err != nil {
				return err
			}
			_, err = part.Write(fileData)
			return err
		},
	)
	if err != nil {
		return "", err
	}
	id, _ := result["id"].(string)
	return id, nil
}

// ── Read receipts ─────────────────────────────────────────────────────────────

// markMessageAsRead sends the read receipt for messageID.
//
// Parameters:
//   - phoneID:   The sender's phone number ID.
//   - messageID: The wamid of the message to mark.
//
// Returns:
//   - error if the API call fails.
func (g *graphAPI) markMessageAsRead(phoneID, messageID string) error {
	_, err := g.post("/"+phoneID+"/messages", map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
	})
	return err
}

// setTypingIndicator shows a typing indicator for the given message.
//
// Parameters:
//   - phoneID:   The sender's phone number ID.
//   - messageID: The wamid of the message to indicate typing on.
//
// Returns:
//   - error if the API call fails.
func (g *graphAPI) setTypingIndicator(phoneID, messageID string) error {
	_, err := g.post("/"+phoneID+"/messages", map[string]any{
		"messaging_product": "whatsapp",
		"status":            "read",
		"message_id":        messageID,
		"typing_indicator":  map[string]any{"type": "text"},
	})
	return err
}

// ── Media retrieval ───────────────────────────────────────────────────────────

// getMediaURL fetches the temporary download URL for a media ID.
//
// Parameters:
//   - mediaID: The WhatsApp media ID.
//
// Returns:
//   - map[string]any with keys id, url, mime_type, sha256, file_size.
//   - error if the API call fails.
func (g *graphAPI) getMediaURL(mediaID string) (map[string]any, error) {
	return g.get("/"+mediaID, nil)
}

// downloadMedia downloads raw bytes from a WhatsApp media URL.
// The URL is authenticated with the bearer token because WhatsApp media
// URLs require the Authorization header (not just a URL parameter).
//
// Parameters:
//   - mediaURL: the URL returned by getMediaURL.
//
// Returns:
//   - []byte: raw file content.
//   - error if the request fails.
func (g *graphAPI) downloadMedia(mediaURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("downloading media %s: HTTP %d: %s", mediaURL, resp.StatusCode, string(b))
	}
	return io.ReadAll(resp.Body)
}

// deleteMedia deletes an uploaded media object.
//
// Parameters:
//   - mediaID:    The media ID to delete.
//   - phoneID:    Optional phone number ID filter (empty = skip).
//
// Returns:
//   - error if the API call fails.
func (g *graphAPI) deleteMedia(mediaID, phoneID string) error {
	params := map[string]string{}
	if phoneID != "" {
		params["phone_number_id"] = phoneID
	}
	_, err := g.delete("/"+mediaID, params)
	return err
}

// ── Business profile ──────────────────────────────────────────────────────────

// getBusinessProfile fetches the business profile for a phone number ID.
//
// Parameters:
//   - phoneID: the phone number ID.
//   - fields:  comma-separated field names to request.
//
// Returns:
//   - map[string]any: the raw API response.
//   - error if the call fails.
func (g *graphAPI) getBusinessProfile(phoneID, fields string) (map[string]any, error) {
	return g.get("/"+phoneID+"/whatsapp_business_profile", map[string]string{
		"fields": fields,
	})
}

// updateBusinessProfile sets fields on the business profile.
//
// Parameters:
//   - phoneID: the phone number ID.
//   - data:    map of fields to update.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) updateBusinessProfile(phoneID string, data map[string]any) error {
	data["messaging_product"] = "whatsapp"
	_, err := g.post("/"+phoneID+"/whatsapp_business_profile", data)
	return err
}

// ── Phone number & WABA info ──────────────────────────────────────────────────

// getBusinessPhoneNumber fetches a single business phone number's details.
//
// Parameters:
//   - phoneID: the phone number ID.
//   - fields:  comma-separated field names.
//
// Returns:
//   - map[string]any raw response.
//   - error.
func (g *graphAPI) getBusinessPhoneNumber(phoneID, fields string) (map[string]any, error) {
	return g.get("/"+phoneID, map[string]string{"fields": fields})
}

// getWABAInfo fetches WABA-level information.
//
// Parameters:
//   - wabaID: the WABA ID.
//   - fields: comma-separated fields.
//
// Returns:
//   - map[string]any raw response.
//   - error.
func (g *graphAPI) getWABAInfo(wabaID, fields string) (map[string]any, error) {
	return g.get("/"+wabaID, map[string]string{"fields": fields})
}

// ── Templates ─────────────────────────────────────────────────────────────────

// createTemplate creates a new message template.
//
// Parameters:
//   - wabaID:   WhatsApp Business Account ID.
//   - template: JSON-encoded template definition.
//
// Returns:
//   - map[string]any with id, status, category.
//   - error.
func (g *graphAPI) createTemplate(wabaID string, template map[string]any) (map[string]any, error) {
	return g.post("/"+wabaID+"/message_templates", template)
}

// deleteTemplate removes a template by name (and optionally ID).
//
// Parameters:
//   - wabaID:       WABA ID.
//   - templateName: template name.
//   - templateID:   optional specific template ID.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) deleteTemplate(wabaID, templateName, templateID string) error {
	params := map[string]string{"name": templateName}
	if templateID != "" {
		params["hsm_id"] = templateID
	}
	_, err := g.delete("/"+wabaID+"/message_templates", params)
	return err
}

// ── Flows ─────────────────────────────────────────────────────────────────────

// createFlow creates a new flow on the given WABA.
//
// Parameters:
//   - wabaID:     WABA ID.
//   - name:       flow name.
//   - categories: slice of FlowCategory strings.
//
// Returns:
//   - map[string]any with id, status.
//   - error.
func (g *graphAPI) createFlow(wabaID, name string, categories []string) (map[string]any, error) {
	return g.post("/"+wabaID+"/flows", map[string]any{
		"name":       name,
		"categories": categories,
	})
}

// publishFlow transitions a flow to PUBLISHED status.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) publishFlow(flowID string) error {
	_, err := g.post("/"+flowID+"/publish", map[string]any{})
	return err
}

// deleteFlow deletes a DRAFT flow.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) deleteFlow(flowID string) error {
	_, err := g.delete("/"+flowID, nil)
	return err
}

// deprecateFlow marks a published flow as deprecated.
//
// Parameters:
//   - flowID: the flow ID.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) deprecateFlow(flowID string) error {
	_, err := g.post("/"+flowID+"/deprecate", map[string]any{})
	return err
}

// getFlow fetches flow metadata.
//
// Parameters:
//   - flowID: the flow ID.
//   - fields: comma-separated fields.
//
// Returns:
//   - map[string]any raw response.
//   - error.
func (g *graphAPI) getFlow(flowID, fields string) (map[string]any, error) {
	return g.get("/"+flowID, map[string]string{"fields": fields})
}

// getFlows fetches all flows for a WABA.
//
// Parameters:
//   - wabaID:     WABA ID.
//   - fields:     comma-separated fields.
//   - pagination: optional paging params.
//
// Returns:
//   - map[string]any with data array and paging.
//   - error.
func (g *graphAPI) getFlows(wabaID, fields string, pagination map[string]string) (map[string]any, error) {
	params := map[string]string{"fields": fields}
	for k, v := range pagination {
		params[k] = v
	}
	return g.get("/"+wabaID+"/flows", params)
}

// ── QR codes ──────────────────────────────────────────────────────────────────

// createQRCode creates a new QR code.
//
// Parameters:
//   - phoneID:          phone number ID.
//   - prefilledMessage: the message text embedded in the QR code.
//   - imageType:        "PNG" or "SVG".
//
// Returns:
//   - map[string]any with code, prefilled_message, deep_link_url, qr_image_url.
//   - error.
func (g *graphAPI) createQRCode(phoneID, prefilledMessage, imageType string) (map[string]any, error) {
	return g.post("/"+phoneID+"/message_qrdls", map[string]any{
		"prefilled_message": prefilledMessage,
		"generate_qr_image": imageType,
	})
}

// deleteQRCode deletes a QR code.
//
// Parameters:
//   - phoneID: phone number ID.
//   - code:    QR code identifier.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) deleteQRCode(phoneID, code string) error {
	_, err := g.delete("/"+phoneID+"/message_qrdls/"+code, nil)
	return err
}

// ── User blocking ─────────────────────────────────────────────────────────────

// blockUsers blocks a list of phone numbers/WA IDs.
//
// Parameters:
//   - phoneID: business phone ID.
//   - users:   slice of WA IDs to block.
//
// Returns:
//   - map[string]any raw response with added_users / failed_users.
//   - error.
func (g *graphAPI) blockUsers(phoneID string, users []string) (map[string]any, error) {
	items := make([]map[string]any, len(users))
	for i, u := range users {
		items[i] = map[string]any{"user": u}
	}
	return g.post("/"+phoneID+"/block_users", map[string]any{"block_users": items})
}

// unblockUsers unblocks a list of WA IDs.
//
// Parameters:
//   - phoneID: business phone ID.
//   - users:   slice of WA IDs to unblock.
//
// Returns:
//   - map[string]any raw response.
//   - error.
func (g *graphAPI) unblockUsers(phoneID string, users []string) (map[string]any, error) {
	items := make([]map[string]any, len(users))
	for i, u := range users {
		items[i] = map[string]any{"user": u}
	}
	return g.post("/"+phoneID+"/block_users", map[string]any{
		"operation":   "unblock",
		"block_users": items,
	})
}

// ── Calling ───────────────────────────────────────────────────────────────────

// initiateCall starts an outbound call.
//
// Parameters:
//   - phoneID:              business phone ID.
//   - to:                   callee WA ID.
//   - sdp:                  session description map.
//   - bizOpaqueCallbackData: tracker string.
//
// Returns:
//   - map[string]any with call_id.
//   - error.
func (g *graphAPI) initiateCall(phoneID, to string, sdp map[string]any, bizOpaqueCallbackData string) (map[string]any, error) {
	body := map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "call",
		"call": map[string]any{
			"action": "initiate",
			"sdp":    sdp,
		},
	}
	if bizOpaqueCallbackData != "" {
		body["biz_opaque_callback_data"] = bizOpaqueCallbackData
	}
	return g.post("/"+phoneID+"/messages", body)
}

// ── Commerce ──────────────────────────────────────────────────────────────────

// getCommerceSettings fetches commerce settings for a phone number.
func (g *graphAPI) getCommerceSettings(phoneID, fields string) (map[string]any, error) {
	return g.get("/"+phoneID+"/whatsapp_commerce_settings", map[string]string{"fields": fields})
}

// updateCommerceSettings updates commerce settings.
func (g *graphAPI) updateCommerceSettings(phoneID string, data map[string]any) error {
	_, err := g.post("/"+phoneID+"/whatsapp_commerce_settings", data)
	return err
}

// ── Webhook registration ──────────────────────────────────────────────────────

// getAppAccessToken fetches a client_credentials app token.
//
// Parameters:
//   - appID:     the Meta app ID.
//   - appSecret: the app secret.
//
// Returns:
//   - access_token string.
//   - error.
func (g *graphAPI) getAppAccessToken(appID, appSecret string) (string, error) {
	res, err := g.get("/oauth/access_token", map[string]string{
		"grant_type":    "client_credentials",
		"client_id":     appID,
		"client_secret": appSecret,
	})
	if err != nil {
		return "", err
	}
	tok, _ := res["access_token"].(string)
	return tok, nil
}

// setAppCallbackURL subscribes the app to webhook events.
//
// Parameters:
//   - appID:           Meta app ID (numeric).
//   - appAccessToken:  token from getAppAccessToken.
//   - callbackURL:     the public HTTPS endpoint.
//   - verifyToken:     challenge verify string.
//   - fields:          webhook fields to subscribe to.
//
// Returns:
//   - error if the call fails.
func (g *graphAPI) setAppCallbackURL(appID int, appAccessToken, callbackURL, verifyToken string, fields []string) error {
	params := map[string]string{
		"object":       "whatsapp_business_account",
		"callback_url": callbackURL,
		"verify_token": verifyToken,
		"fields":       strings.Join(fields, ","),
		"access_token": appAccessToken,
	}
	_, err := g.request(http.MethodPost, "/"+strconv.Itoa(appID)+"/subscriptions", params, nil, nil)
	return err
}

// ── Registration ──────────────────────────────────────────────────────────────

// registerPhoneNumber registers a phone number with a 2-step PIN.
//
// Parameters:
//   - phoneID:               phone number ID.
//   - pin:                   6-digit 2-step verification PIN.
//   - dataLocalizationRegion: optional ISO-3166 country code for local storage.
//
// Returns:
//   - error.
func (g *graphAPI) registerPhoneNumber(phoneID, pin, dataLocalizationRegion string) error {
	body := map[string]any{
		"messaging_product": "whatsapp",
		"pin":               pin,
	}
	if dataLocalizationRegion != "" {
		body["data_localization_region"] = dataLocalizationRegion
	}
	_, err := g.post("/"+phoneID+"/register", body)
	return err
}

// deregisterPhoneNumber deregisters a phone number.
//
// Parameters:
//   - phoneID: the phone number ID.
//
// Returns:
//   - error.
func (g *graphAPI) deregisterPhoneNumber(phoneID string) error {
	_, err := g.post("/"+phoneID+"/deregister", map[string]any{})
	return err
}

// ── Display name ──────────────────────────────────────────────────────────────

// updateDisplayName updates the business display name.
//
// Parameters:
//   - phoneID:        phone number ID.
//   - newDisplayName: new display name string.
//
// Returns:
//   - error.
func (g *graphAPI) updateDisplayName(phoneID, newDisplayName string) error {
	_, err := g.post("/"+phoneID, map[string]any{"new_business_name": newDisplayName})
	return err
}

// ── Conversational automation ─────────────────────────────────────────────────

// updateConversationalAutomation updates ice-breakers and slash-commands.
//
// Parameters:
//   - phoneID:              phone number ID.
//   - enableWelcomeMessage: whether to enable the chat-opened webhook.
//   - prompts:              ice-breaker strings.
//   - commands:             JSON-encoded commands array.
//
// Returns:
//   - error.
func (g *graphAPI) updateConversationalAutomation(
	phoneID string,
	enableWelcomeMessage bool,
	prompts []string,
	commands string,
) error {
	body := map[string]any{
		"enable_welcome_message": enableWelcomeMessage,
	}
	if len(prompts) > 0 {
		body["prompts"] = prompts
	}
	if commands != "" {
		body["commands"] = commands
	}
	_, err := g.post("/"+phoneID+"/conversational_automation", body)
	return err
}

// ── Blocked users list ────────────────────────────────────────────────────────

// getBlockedUsers paginates through blocked users.
//
// Parameters:
//   - phoneID:    phone number ID.
//   - pagination: optional cursor params.
//
// Returns:
//   - map[string]any with data array and paging.
//   - error.
func (g *graphAPI) getBlockedUsers(phoneID string, pagination map[string]string) (map[string]any, error) {
	params := map[string]string{}
	for k, v := range pagination {
		params[k] = v
	}
	return g.get("/"+phoneID+"/block_users", params)
}

// ── Typing indicator (alias of markAsRead) ────────────────────────────────────

// indicateTyping shows the typing indicator for a message.
// Internally identical to markMessageAsRead with an additional indicator type field.
//
// Parameters:
//   - phoneID:   business phone ID.
//   - messageID: wamid to indicate typing on.
//
// Returns:
//   - error.
func (g *graphAPI) indicateTyping(phoneID, messageID string) error {
	return g.setTypingIndicator(phoneID, messageID)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// toFloat64 safely converts interface{} → float64.
// Used when decoding JSON numbers (which Go unmarshals as float64).
func toFloat64(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// toString safely converts interface{} → string.
func toString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// toBool safely converts interface{} → bool.
func toBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// toStringSlice safely converts []interface{} → []string.
func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(arr))
	for i, item := range arr {
		out[i] = fmt.Sprint(item)
	}
	return out
}

// toTime parses a unix epoch number (float64 or string) to time.Time (UTC).
func toTime(v any) time.Time {
	switch n := v.(type) {
	case float64:
		return time.Unix(int64(n), 0).UTC()
	case string:
		ts, err := strconv.ParseInt(n, 10, 64)
		if err == nil {
			return time.Unix(ts, 0).UTC()
		}
	}
	return time.Time{}
}
