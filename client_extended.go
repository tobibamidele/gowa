package gowa

import (
	"fmt"
	"mime/multipart"
	"time"
)

// multipartWriter is an alias so client_extended.go can reference the concrete type
// used by the api.go request() method without re-importing mime/multipart everywhere.
type multipartWriter = multipart.Writer

// ────────────────────────────────────────────────────────────────────────────────
// Extended client methods
// Covers every pywa client method not yet implemented in client.go.
// ────────────────────────────────────────────────────────────────────────────────

// ── send_products ─────────────────────────────────────────────────────────────

// SendProductsOptions holds optional parameters for SendProducts.
type SendProductsOptions struct {
	Footer           string
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
}

// SendProducts sends a multi-section product list message.
// Mirrors pywa's WhatsApp.send_products.
//
// Parameters:
//   - to:             recipient WA ID.
//   - catalogID:      the catalog ID.
//   - title:          header text (up to 60 chars).
//   - body:           body text (up to 1024 chars).
//   - sections:       up to 10 ProductsSection values (max 30 products total).
//   - opts:           optional SendProductsOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) SendProducts(to, catalogID, title, body string, sections []ProductsSection, opts ...SendProductsOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SendProductsOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	sectionDicts := make([]map[string]any, len(sections))
	for i, s := range sections {
		sectionDicts[i] = s.toDict()
	}

	interactive := map[string]any{
		"type": "product_list",
		"header": map[string]any{
			"type": "text",
			"text": title,
		},
		"body": map[string]any{"text": body},
		"action": map[string]any{
			"catalog_id": catalogID,
			"sections":   sectionDicts,
		},
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

// ── Business account / phone number info ──────────────────────────────────────

// GetBusinessAccount fetches WhatsApp Business Account (WABA) information.
// Mirrors pywa's WhatsApp.get_business_account.
//
// Parameters:
//   - wabaID: optional WABA ID override.
//
// Returns:
//   - *WhatsAppBusinessAccount.
//   - error.
func (wa *WhatsApp) GetBusinessAccount(wabaID string) (*WhatsAppBusinessAccount, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.getWABAInfo(wid, "id,name,currency,message_template_namespace")
	if err != nil {
		return nil, err
	}
	return &WhatsAppBusinessAccount{
		ID:                       toString(res["id"]),
		Name:                     toString(res["name"]),
		Currency:                 toString(res["currency"]),
		MessageTemplateNamespace: toString(res["message_template_namespace"]),
	}, nil
}

// GetBusinessPhoneNumber fetches details about a business phone number.
// Mirrors pywa's WhatsApp.get_business_phone_number.
//
// Parameters:
//   - phoneID: optional override.
//
// Returns:
//   - *BusinessPhoneNumber.
//   - error.
func (wa *WhatsApp) GetBusinessPhoneNumber(phoneID string) (*BusinessPhoneNumber, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	fields := "id,display_phone_number,verified_name,quality_rating,code_verification_status,name_status,is_official_business_account,account_mode"
	res, err := wa.api.getBusinessPhoneNumber(pid, fields)
	if err != nil {
		return nil, err
	}
	return &BusinessPhoneNumber{
		ID:                     toString(res["id"]),
		DisplayPhoneNumber:     toString(res["display_phone_number"]),
		VerifiedName:           toString(res["verified_name"]),
		QualityRating:          toString(res["quality_rating"]),
		CodeVerificationStatus: toString(res["code_verification_status"]),
		NameStatus:             toString(res["name_status"]),
		IsOfficialBizAcct:      toBool(res["is_official_business_account"]),
		AccountMode:            toString(res["account_mode"]),
	}, nil
}

// GetBusinessPhoneNumbers fetches all phone numbers for a WABA.
// Mirrors pywa's WhatsApp.get_business_phone_numbers.
//
// Parameters:
//   - wabaID:     optional WABA ID override.
//   - pagination: optional cursor paging.
//
// Returns:
//   - *Result[*BusinessPhoneNumber].
//   - error.
func (wa *WhatsApp) GetBusinessPhoneNumbers(wabaID string, pagination *Pagination) (*Result[*BusinessPhoneNumber], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	fields := "id,display_phone_number,verified_name,quality_rating"
	pg := map[string]string{"fields": fields}
	if pagination != nil {
		for k, v := range pagination.toDict() {
			pg[k] = v
		}
	}
	res, err := wa.api.get("/"+wid+"/phone_numbers", pg)
	if err != nil {
		return nil, err
	}
	r := &Result[*BusinessPhoneNumber]{}
	for _, d := range toAnySlice(res["data"]) {
		dm := toMap(d)
		if dm == nil {
			continue
		}
		r.Items = append(r.Items, &BusinessPhoneNumber{
			ID:                 toString(dm["id"]),
			DisplayPhoneNumber: toString(dm["display_phone_number"]),
			VerifiedName:       toString(dm["verified_name"]),
			QualityRating:      toString(dm["quality_rating"]),
		})
	}
	if paging := toMap(res["paging"]); paging != nil {
		if cursors := toMap(paging["cursors"]); cursors != nil {
			r.NextCursor = toString(cursors["after"])
		}
	}
	return r, nil
}

// SetBusinessPublicKey sets the RSA public key for end-to-end encryption in flows.
// Mirrors pywa's WhatsApp.set_business_public_key.
//
// Parameters:
//   - publicKey: 2048-bit RSA public key in PEM format.
//   - phoneID:   optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) SetBusinessPublicKey(publicKey, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.post("/"+pid+"/whatsapp_business_encryption", map[string]any{
		"business_public_key": publicKey,
	})
	return apiErr
}

// ── Template management ───────────────────────────────────────────────────────

// TemplateDetails holds full metadata for a retrieved template.
type TemplateDetails struct {
	ID           string
	Name         string
	Status       TemplateStatus
	Category     TemplateCategory
	Language     string
	Components   []map[string]any
	QualityScore string
}

// GetTemplate fetches the details of a specific template.
// Mirrors pywa's WhatsApp.get_template.
//
// Parameters:
//   - templateID: the template ID.
//
// Returns:
//   - *TemplateDetails.
//   - error.
func (wa *WhatsApp) GetTemplate(templateID string) (*TemplateDetails, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	fields := "id,name,status,category,language,components,quality_score"
	res, err := wa.api.get("/"+templateID, map[string]string{"fields": fields})
	if err != nil {
		return nil, err
	}
	return parseTemplateDetails(res), nil
}

// GetTemplatesOptions holds filter parameters for GetTemplates.
type GetTemplatesOptions struct {
	Status     string
	Category   string
	Language   string
	Name       string
	Pagination *Pagination
	WABAOID    string
}

// GetTemplates fetches templates for a WABA with optional filters.
// Mirrors pywa's WhatsApp.get_templates.
//
// Parameters:
//   - opts: optional GetTemplatesOptions.
//
// Returns:
//   - *Result[*TemplateDetails].
//   - error.
func (wa *WhatsApp) GetTemplates(opts ...GetTemplatesOptions) (*Result[*TemplateDetails], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := GetTemplatesOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	wid, err := wa.resolveWABAID(o.WABAOID)
	if err != nil {
		return nil, err
	}

	params := map[string]string{
		"fields": "id,name,status,category,language,components,quality_score",
	}
	if o.Status != "" {
		params["status"] = o.Status
	}
	if o.Category != "" {
		params["category"] = o.Category
	}
	if o.Language != "" {
		params["language"] = o.Language
	}
	if o.Name != "" {
		params["name"] = o.Name
	}
	if o.Pagination != nil {
		for k, v := range o.Pagination.toDict() {
			params[k] = v
		}
	}

	res, err := wa.api.get("/"+wid+"/message_templates", params)
	if err != nil {
		return nil, err
	}
	r := &Result[*TemplateDetails]{}
	for _, d := range toAnySlice(res["data"]) {
		if dm := toMap(d); dm != nil {
			r.Items = append(r.Items, parseTemplateDetails(dm))
		}
	}
	if paging := toMap(res["paging"]); paging != nil {
		if cursors := toMap(paging["cursors"]); cursors != nil {
			r.NextCursor = toString(cursors["after"])
		}
	}
	return r, nil
}

// UpdateTemplateOptions holds parameters for UpdateTemplate.
type UpdateTemplateOptions struct {
	NewCategory              TemplateCategory
	NewComponents            []map[string]any
	NewMessageSendTTLSeconds int
}

// UpdateTemplate modifies an existing message template.
// Mirrors pywa's WhatsApp.update_template.
//
// Parameters:
//   - templateID: the template ID.
//   - opts:       fields to update.
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateTemplate(templateID string, opts UpdateTemplateOptions) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	body := map[string]any{}
	if opts.NewCategory != "" {
		body["category"] = string(opts.NewCategory)
	}
	if opts.NewComponents != nil {
		body["components"] = opts.NewComponents
	}
	if opts.NewMessageSendTTLSeconds > 0 {
		body["message_send_ttl_seconds"] = opts.NewMessageSendTTLSeconds
	}
	_, err := wa.api.post("/"+templateID, body)
	return err
}

// CompareTemplates compares two templates' performance metrics.
// Mirrors pywa's WhatsApp.compare_templates.
//
// Parameters:
//   - templateID:  first template ID.
//   - templateID2: second template ID.
//   - start:       start of the comparison period.
//   - end:         end of the comparison period.
//
// Returns:
//   - map[string]any: raw comparison data from the API.
//   - error.
func (wa *WhatsApp) CompareTemplates(templateID, templateID2 string, start, end time.Time) (map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	return wa.api.get("/"+templateID+"/template_analytics", map[string]string{
		"compare_to": templateID2,
		"start":      fmt.Sprintf("%d", start.Unix()),
		"end":        fmt.Sprintf("%d", end.Unix()),
	})
}

// UnpauseTemplate lifts a pacing-induced pause on a template.
// Mirrors pywa's WhatsApp.unpause_template.
//
// Parameters:
//   - templateID: the template ID.
//
// Returns:
//   - error.
func (wa *WhatsApp) UnpauseTemplate(templateID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	_, err := wa.api.post("/"+templateID, map[string]any{"pause_until": "resume"})
	return err
}

// MigrateTemplates migrates approved templates from one WABA to another.
// Mirrors pywa's WhatsApp.migrate_templates.
//
// Parameters:
//   - sourceWABAID:      WABA ID to migrate from.
//   - pageNumber:        zero-indexed page (each page = 500 templates); -1 = first page.
//   - destinationWABAID: WABA ID to migrate to (uses Config.BusinessAccountID if empty).
//
// Returns:
//   - map[string]any: raw migration result.
//   - error.
func (wa *WhatsApp) MigrateTemplates(sourceWABAID string, pageNumber int, destinationWABAID string) (map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	destID, err := wa.resolveWABAID(destinationWABAID)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"source_waba_id": sourceWABAID,
	}
	if pageNumber >= 0 {
		body["page_number"] = pageNumber
	}
	return wa.api.post("/"+destID+"/message_templates_migration", body)
}

// parseTemplateDetails decodes a raw API map into TemplateDetails.
func parseTemplateDetails(m map[string]any) *TemplateDetails {
	td := &TemplateDetails{
		ID:       toString(m["id"]),
		Name:     toString(m["name"]),
		Status:   TemplateStatus(toString(m["status"])),
		Category: TemplateCategory(toString(m["category"])),
		Language: toString(m["language"]),
	}
	if qs := toMap(m["quality_score"]); qs != nil {
		td.QualityScore = toString(qs["score"])
	}
	for _, c := range toAnySlice(m["components"]) {
		if cm := toMap(c); cm != nil {
			td.Components = append(td.Components, cm)
		}
	}
	return td
}

// ── Flow JSON update ──────────────────────────────────────────────────────────

// UpdateFlowJSON uploads new JSON for an existing flow.
// Mirrors pywa's WhatsApp.update_flow_json.
//
// Parameters:
//   - flowID:   the flow ID.
//   - flowJSON: JSON content as a string, []byte, or map[string]any.
//
// Returns:
//   - validationErrors: slice of validation error maps (empty = no errors).
//   - error.
func (wa *WhatsApp) UpdateFlowJSON(flowID string, flowJSON any) ([]map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	var jsonStr string
	switch v := flowJSON.(type) {
	case string:
		jsonStr = v
	case []byte:
		jsonStr = string(v)
	case map[string]any:
		b, err := marshalJSON(v)
		if err != nil {
			return nil, err
		}
		jsonStr = string(b)
	default:
		return nil, fmt.Errorf("UpdateFlowJSON: unsupported type %T", flowJSON)
	}

	// The Graph API expects a multipart/form-data upload for flow assets.
	result, err := wa.api.request("POST", "/"+flowID+"/assets", nil, nil,
		func(mw *multipartWriter) error {
			if err := mw.WriteField("asset_type", "FLOW_JSON"); err != nil {
				return err
			}
			if err := mw.WriteField("name", "flow.json"); err != nil {
				return err
			}
			part, err := mw.CreateFormFile("file", "flow.json")
			if err != nil {
				return err
			}
			_, err = part.Write([]byte(jsonStr))
			return err
		},
	)
	if err != nil {
		return nil, err
	}

	var validationErrors []map[string]any
	for _, ve := range toAnySlice(result["validation_errors"]) {
		if vm := toMap(ve); vm != nil {
			validationErrors = append(validationErrors, vm)
		}
	}
	return validationErrors, nil
}

// UpdateFlowMetadataOptions holds fields for UpdateFlowMetadata.
type UpdateFlowMetadataOptions struct {
	Name        string
	Categories  []FlowCategory
	EndpointURI string
}

// UpdateFlowMetadata updates the name, categories, or endpoint of an existing flow.
// Mirrors pywa's WhatsApp.update_flow_metadata.
//
// Parameters:
//   - flowID: the flow ID.
//   - opts:   fields to update (at least one must be non-zero).
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateFlowMetadata(flowID string, opts UpdateFlowMetadataOptions) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	body := map[string]any{}
	if opts.Name != "" {
		body["name"] = opts.Name
	}
	if len(opts.Categories) > 0 {
		cats := make([]string, len(opts.Categories))
		for i, c := range opts.Categories {
			cats[i] = string(c)
		}
		body["categories"] = cats
	}
	if opts.EndpointURI != "" {
		body["endpoint_uri"] = opts.EndpointURI
	}
	if len(body) == 0 {
		return fmt.Errorf("gowa: UpdateFlowMetadata requires at least one field")
	}
	_, err := wa.api.post("/"+flowID, body)
	return err
}

// GetFlowAssets returns the assets (JSON, images) attached to a flow.
// Mirrors pywa's WhatsApp.get_flow_assets.
//
// Parameters:
//   - flowID:     the flow ID.
//   - pagination: optional cursor paging.
//
// Returns:
//   - []map[string]any: list of asset objects from the API.
//   - error.
func (wa *WhatsApp) GetFlowAssets(flowID string, pagination *Pagination) ([]map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	params := map[string]string{}
	if pagination != nil {
		for k, v := range pagination.toDict() {
			params[k] = v
		}
	}
	res, err := wa.api.get("/"+flowID+"/assets", params)
	if err != nil {
		return nil, err
	}
	var assets []map[string]any
	for _, d := range toAnySlice(res["data"]) {
		if dm := toMap(d); dm != nil {
			assets = append(assets, dm)
		}
	}
	return assets, nil
}

// MigrateFlows migrates named flows between WABAs.
// Mirrors pywa's WhatsApp.migrate_flows.
//
// Parameters:
//   - sourceWABAID:      source WABA ID.
//   - sourceFlowNames:   list of flow names to migrate.
//   - destinationWABAID: destination WABA ID (uses Config.BusinessAccountID if empty).
//
// Returns:
//   - map[string]any: raw migration result.
//   - error.
func (wa *WhatsApp) MigrateFlows(sourceWABAID string, sourceFlowNames []string, destinationWABAID string) (map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	destID, err := wa.resolveWABAID(destinationWABAID)
	if err != nil {
		return nil, err
	}
	return wa.api.post("/"+destID+"/flows_migration", map[string]any{
		"source_waba_id":    sourceWABAID,
		"source_flow_names": sourceFlowNames,
	})
}

// ── QR code — get and update ──────────────────────────────────────────────────

// GetQRCode fetches a single QR code by its code string.
// Mirrors pywa's WhatsApp.get_qr_code.
//
// Parameters:
//   - code:      the QR code identifier.
//   - imageType: "PNG" | "SVG" | "" (omit to skip image URL).
//   - phoneID:   optional override.
//
// Returns:
//   - *QRCode, or nil if not found.
//   - error.
func (wa *WhatsApp) GetQRCode(code, imageType, phoneID string) (*QRCode, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	if imageType != "" {
		params["generate_qr_image"] = imageType
	}
	res, err := wa.api.get("/"+pid+"/message_qrdls/"+code, params)
	if err != nil {
		return nil, err
	}
	data := toAnySlice(res["data"])
	if len(data) == 0 {
		return nil, nil
	}
	dm := toMap(data[0])
	return &QRCode{
		Code:             toString(dm["code"]),
		PrefilledMessage: toString(dm["prefilled_message"]),
		DeepLinkURL:      toString(dm["deep_link_url"]),
		QRImageURL:       toString(dm["qr_image_url"]),
		client:           wa,
		phoneID:          pid,
	}, nil
}

// GetQRCodes fetches all QR codes for a phone number.
// Mirrors pywa's WhatsApp.get_qr_codes.
//
// Parameters:
//   - imageType:  "PNG" | "SVG" | "" (omit to skip image URLs).
//   - phoneID:    optional override.
//   - pagination: optional cursor paging.
//
// Returns:
//   - *Result[*QRCode].
//   - error.
func (wa *WhatsApp) GetQRCodes(imageType, phoneID string, pagination *Pagination) (*Result[*QRCode], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	if imageType != "" {
		params["generate_qr_image"] = imageType
	}
	if pagination != nil {
		for k, v := range pagination.toDict() {
			params[k] = v
		}
	}
	res, err := wa.api.get("/"+pid+"/message_qrdls", params)
	if err != nil {
		return nil, err
	}
	r := &Result[*QRCode]{}
	for _, d := range toAnySlice(res["data"]) {
		dm := toMap(d)
		if dm == nil {
			continue
		}
		r.Items = append(r.Items, &QRCode{
			Code:             toString(dm["code"]),
			PrefilledMessage: toString(dm["prefilled_message"]),
			DeepLinkURL:      toString(dm["deep_link_url"]),
			QRImageURL:       toString(dm["qr_image_url"]),
			client:           wa,
			phoneID:          pid,
		})
	}
	return r, nil
}

// UpdateQRCode updates the prefilled message of an existing QR code.
// Mirrors pywa's WhatsApp.update_qr_code.
//
// Parameters:
//   - code:             QR code identifier.
//   - prefilledMessage: new message text.
//   - phoneID:          optional override.
//
// Returns:
//   - *QRCode with updated values.
//   - error.
func (wa *WhatsApp) UpdateQRCode(code, prefilledMessage, phoneID string) (*QRCode, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.post("/"+pid+"/message_qrdls/"+code, map[string]any{
		"prefilled_message": prefilledMessage,
	})
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

// ── WABA / phone callback URL overrides ───────────────────────────────────────

// OverrideWABACallbackURL sets an alternate webhook URL at the WABA level.
// Mirrors pywa's WhatsApp.override_waba_callback_url.
//
// Parameters:
//   - callbackURL: public HTTPS URL.
//   - verifyToken: challenge string.
//   - wabaID:      optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) OverrideWABACallbackURL(callbackURL, verifyToken, wabaID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.post("/"+wid+"/subscribed_apps", map[string]any{
		"override_callback_uri": callbackURL,
		"verify_token":          verifyToken,
	})
	return apiErr
}

// DeleteWABACallbackURL removes the WABA-level alternate callback URL.
// Mirrors pywa's WhatsApp.delete_waba_callback_url.
//
// Parameters:
//   - wabaID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeleteWABACallbackURL(wabaID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.delete("/"+wid+"/subscribed_apps", nil)
	return apiErr
}

// OverridePhoneCallbackURL sets an alternate webhook URL at the phone-number level.
// Mirrors pywa's WhatsApp.override_phone_callback_url.
//
// Parameters:
//   - callbackURL: public HTTPS URL.
//   - verifyToken: challenge string.
//   - phoneID:     optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) OverridePhoneCallbackURL(callbackURL, verifyToken, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.post("/"+pid+"/alternate_callback_uri", map[string]any{
		"override_callback_uri": callbackURL,
		"verify_token":          verifyToken,
	})
	return apiErr
}

// DeletePhoneCallbackURL removes the phone-number-level alternate callback URL.
// Mirrors pywa's WhatsApp.delete_phone_callback_url.
//
// Parameters:
//   - phoneID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeletePhoneCallbackURL(phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.delete("/"+pid+"/alternate_callback_uri", nil)
	return apiErr
}

// ── Calling — pre-accept / accept / reject / terminate ────────────────────────

// PreAcceptCall pre-accepts an inbound call to pre-establish the media path.
// Mirrors pywa's WhatsApp.pre_accept_call.
//
// Parameters:
//   - callID:  call ID from CallConnect.
//   - sdp:     session description.
//   - phoneID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) PreAcceptCall(callID string, sdp SessionDescription, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.post("/"+pid+"/calls/"+callID, map[string]any{
		"action": "pre_accept",
		"sdp":    sdp.toDict(),
	})
	return apiErr
}

// AcceptCall accepts an inbound call.
// Mirrors pywa's WhatsApp.accept_call.
//
// Parameters:
//   - callID:   call ID from CallConnect.
//   - sdp:      session description.
//   - tracker:  optional opaque tracker string.
//   - phoneID:  optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) AcceptCall(callID string, sdp SessionDescription, tracker, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	body := map[string]any{
		"action": "accept",
		"sdp":    sdp.toDict(),
	}
	if tracker != "" {
		body["biz_opaque_callback_data"] = tracker
	}
	_, apiErr := wa.api.post("/"+pid+"/calls/"+callID, body)
	return apiErr
}

// RejectCall rejects an inbound call.
// Mirrors pywa's WhatsApp.reject_call.
//
// Parameters:
//   - callID:  call ID from CallConnect.
//   - phoneID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) RejectCall(callID, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.post("/"+pid+"/calls/"+callID, map[string]any{"action": "reject"})
	return apiErr
}

// TerminateCall terminates an active call.
// Mirrors pywa's WhatsApp.terminate_call.
//
// Parameters:
//   - callID:  call ID.
//   - phoneID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) TerminateCall(callID, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.post("/"+pid+"/calls/"+callID, map[string]any{"action": "terminate"})
	return apiErr
}

// GetCallPermissions fetches the calling permissions for a user.
// Mirrors pywa's WhatsApp.get_call_permissions.
//
// Parameters:
//   - waID:    the user's WhatsApp ID.
//   - phoneID: optional override.
//
// Returns:
//   - *CallPermissionsResult with the permission status.
//   - error.
func (wa *WhatsApp) GetCallPermissions(waID, phoneID string) (*CallPermissionsResult, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.get("/"+pid+"/call_permissions/"+waID, nil)
	if err != nil {
		return nil, err
	}
	return &CallPermissionsResult{
		Status:   toString(res["status"]),
		UserWAID: toString(res["user_wa_id"]),
		PhoneID:  pid,
	}, nil
}

// CallPermissionsResult holds the result of GetCallPermissions.
type CallPermissionsResult struct {
	Status   string
	UserWAID string
	PhoneID  string
}

// ── GetBlockedUsers ───────────────────────────────────────────────────────────

// GetBlockedUsers returns a paginated list of users blocked from this phone number.
// Mirrors pywa's WhatsApp.get_blocked_users.
//
// Parameters:
//   - phoneID:    optional override.
//   - pagination: optional cursor paging.
//
// Returns:
//   - *Result[*User].
//   - error.
func (wa *WhatsApp) GetBlockedUsers(phoneID string, pagination *Pagination) (*Result[*User], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	pg := map[string]string{}
	if pagination != nil {
		pg = pagination.toDict()
	}
	res, err := wa.api.getBlockedUsers(pid, pg)
	if err != nil {
		return nil, err
	}
	r := &Result[*User]{}
	for _, d := range toAnySlice(res["data"]) {
		dm := toMap(d)
		if dm == nil {
			continue
		}
		r.Items = append(r.Items, &User{
			WAID:   toString(dm["wa_id"]),
			client: wa,
		})
	}
	return r, nil
}

// ── UpsertAuthenticationTemplate ─────────────────────────────────────────────

// AuthOTPButtonType specifies the type of OTP button in an authentication template.
type AuthOTPButtonType string

const (
	AuthOTPButtonCopyCode AuthOTPButtonType = "OTP"
	AuthOTPButtonOneTap   AuthOTPButtonType = "ONE_TAP"
	AuthOTPButtonZeroTap  AuthOTPButtonType = "ZERO_TAP"
)

// UpsertAuthTemplateOptions holds parameters for UpsertAuthenticationTemplate.
type UpsertAuthTemplateOptions struct {
	// Languages is the list of BCP-47 language codes to create/update the template in.
	Languages []string
	// OTPButtonType is the type of OTP button (copy-code, one-tap, zero-tap).
	OTPButtonType AuthOTPButtonType
	// AddSecurityRecommendation adds a "don't share this code" notice.
	AddSecurityRecommendation bool
	// CodeExpirationMinutes adds a code-expiry notice.
	CodeExpirationMinutes int
	// WABAOID overrides the client WABA ID.
	WABAOID string
}

// UpsertAuthenticationTemplate bulk-creates or updates authentication templates
// across multiple languages.
// Mirrors pywa's WhatsApp.upsert_authentication_template.
//
// Parameters:
//   - name: template name (must be unique within the WABA).
//   - opts: UpsertAuthTemplateOptions.
//
// Returns:
//   - []CreatedTemplate: one entry per language.
//   - error.
func (wa *WhatsApp) UpsertAuthenticationTemplate(name string, opts UpsertAuthTemplateOptions) ([]CreatedTemplate, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(opts.WABAOID)
	if err != nil {
		return nil, err
	}

	otpButton := map[string]any{"type": string(opts.OTPButtonType)}
	components := []map[string]any{
		{"type": "BUTTONS", "buttons": []map[string]any{otpButton}},
	}
	if opts.AddSecurityRecommendation {
		components = append(components, map[string]any{
			"type":                        "BODY",
			"add_security_recommendation": true,
		})
	}
	if opts.CodeExpirationMinutes > 0 {
		components = append(components, map[string]any{
			"type":                    "FOOTER",
			"code_expiration_minutes": opts.CodeExpirationMinutes,
		})
	}

	body := map[string]any{
		"name":       name,
		"category":   "AUTHENTICATION",
		"languages":  opts.Languages,
		"components": components,
	}

	res, err := wa.api.post("/"+wid+"/upsert_message_templates", body)
	if err != nil {
		return nil, err
	}

	var results []CreatedTemplate
	for _, d := range toAnySlice(res["data"]) {
		dm := toMap(d)
		if dm == nil {
			continue
		}
		results = append(results, CreatedTemplate{
			ID:       toString(dm["id"]),
			Status:   TemplateStatus(toString(dm["status"])),
			Category: TemplateCategoryAuthentication,
		})
	}
	return results, nil
}

// ── GetBusinessAccessToken ────────────────────────────────────────────────────

// GetBusinessAccessToken exchanges an Embedded Signup code for a business token.
// Mirrors pywa's GraphAPI.get_business_access_token / pywa embedded signup flow.
//
// Parameters:
//   - appID:     the Meta app ID.
//   - appSecret: the app secret.
//   - code:      the code returned by Embedded Signup.
//
// Returns:
//   - token string.
//   - error.
func (wa *WhatsApp) GetBusinessAccessToken(appID, appSecret, code string) (string, error) {
	if err := wa.requireAPI(); err != nil {
		return "", err
	}
	res, err := wa.api.get("/oauth/access_token", map[string]string{
		"client_id":     appID,
		"client_secret": appSecret,
		"code":          code,
	})
	if err != nil {
		return "", err
	}
	tok, _ := res["business_token"].(string)
	if tok == "" {
		tok, _ = res["access_token"].(string)
	}
	return tok, nil
}

// ── marshalJSON helper ────────────────────────────────────────────────────────

// marshalJSON is a thin wrapper around encoding/json.Marshal.
// Declared here to keep imports clean.
func marshalJSON(v any) ([]byte, error) {
	// encoding/json is already imported in api.go (same package)
	return jsonMarshal(v)
}
