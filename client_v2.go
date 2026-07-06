package gowa

import (
	"fmt"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────────
// client_v2.go — new public client methods added in pywa 4.1.0 – 4.2.1.
//
// New surface area:
//   • SendCarousel           — horizontal scrollable media cards
//   • RequestContactInfo     — prompt user to share their phone number
//   • SendRawRequest         — escape hatch for unsupported endpoints
//   • Groups                 — full group CRUD + join-request management
//   • Username               — set/get/delete business usernames
//   • Phone provisioning     — CreatePhoneNumber, RequestVerificationCode, VerifyPhoneNumber
//   • WABA portfolio         — GetSharedBusinessAccounts, GetOwnedBusinessAccounts,
//                              UpdateBusinessAccountSettings, GetWABASubscribedApps
//   • Template archive       — ArchiveTemplates, UnarchiveTemplates
//   • PinMessage / UnpinMessage
// ────────────────────────────────────────────────────────────────────────────────

// ── SendCarousel ──────────────────────────────────────────────────────────────

// SendCarouselOptions holds optional parameters for SendCarousel.
type SendCarouselOptions struct {
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
}

// SendCarousel sends a horizontally scrollable carousel of up to 10 media cards.
// Each card can have an image or video header, optional body text, and up to 3
// quick-reply buttons or one CTA URL button.
// Mirrors pywa's WhatsApp.send_carousel.
//
// Parameters:
//   - to:   recipient WA ID.
//   - body: message body text shown above the carousel (max 1024 chars).
//   - cards: up to 10 ImageCarouselCard or VideoCarouselCard values.
//   - opts: optional SendCarouselOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
//
// Example:
//
//	wa.SendCarousel("2348012345678", "Pick your plan 👇", []gowa.CarouselCard{
//	    gowa.ImageCarouselCard{
//	        Image:   "https://example.com/starter.jpg",
//	        Body:    "Starter – ₦5,000/mo",
//	        Buttons: []gowa.Button{{ID: "plan_starter", Title: "Choose Starter"}},
//	    },
//	    gowa.ImageCarouselCard{
//	        Image:   "https://example.com/pro.jpg",
//	        Body:    "Pro – ₦15,000/mo",
//	        Buttons: []gowa.Button{{ID: "plan_pro", Title: "Choose Pro"}},
//	    },
//	})
func (wa *WhatsApp) SendCarousel(to, body string, cards []CarouselCard, opts ...SendCarouselOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	if len(cards) == 0 || len(cards) > 10 {
		return nil, fmt.Errorf("gowa: SendCarousel requires 1–10 cards, got %d", len(cards))
	}
	o := SendCarouselOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}

	cardDicts := make([]map[string]any, len(cards))
	for i, c := range cards {
		cardDicts[i] = c.toCarouselDict(i)
	}

	payload := sendMessagePayload{
		To:   to,
		Type: "interactive",
		Interactive: map[string]any{
			"type": "carousel",
			"body": map[string]any{"text": body},
			"action": map[string]any{
				"cards": cardDicts,
			},
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

// ── RequestContactInfo ────────────────────────────────────────────────────────

// RequestContactInfoOptions holds optional parameters for RequestContactInfo.
type RequestContactInfoOptions struct {
	ReplyToMessageID string
	Tracker          string
	IdentityKeyHash  string
	Sender           string
}

// SentContactInfoRequest is returned by RequestContactInfo.
// Use WaitForContactInfo to block until the user shares their contact.
type SentContactInfoRequest struct {
	SentMessage
}

// RequestContactInfo sends an interactive message with a "Share Phone Number"
// button. When the user taps it, a contacts webhook fires with their WA phone
// number. Ideal for bots that need to verify or capture the user's number.
// Mirrors pywa's WhatsApp.request_contact_info.
//
// Parameters:
//   - to:   recipient WA ID.
//   - text: body text shown with the "Share" button.
//   - opts: optional RequestContactInfoOptions.
//
// Returns:
//   - *SentContactInfoRequest.
//   - error.
func (wa *WhatsApp) RequestContactInfo(to, text string, opts ...RequestContactInfoOptions) (*SentContactInfoRequest, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := RequestContactInfoOptions{}
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
			"type": "request_contact_info",
			"body": map[string]any{"text": text},
			"action": map[string]any{
				"name": "request_contact_info",
			},
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
	return &SentContactInfoRequest{SentMessage: *sm}, nil
}

// WaitForContactInfo blocks until the user shares their contact (a contacts
// message with their phone number) or the timeout expires.
//
// Parameters:
//   - wa:      the active client.
//   - timeout: maximum wait; 0 = no timeout.
//
// Returns:
//   - *Message with msg.Contacts populated.
//   - error: *ListenerTimeout, *ListenerCanceled, or *ListenerStopped.
func (s *SentContactInfoRequest) WaitForContactInfo(wa *WhatsApp, timeout ...time.Duration) (*Message, error) {
	var dur time.Duration
	if len(timeout) > 0 {
		dur = timeout[0]
	}
	return wa.Listen(ListenOptions{
		SenderWAID:  s.To,
		RecipientID: s.FromPhoneID,
		Filters:     FilterContacts,
		Timeout:     dur,
	})
}

// ── SendRawRequest ────────────────────────────────────────────────────────────

// SendRawRequest sends an arbitrary request to the WhatsApp Cloud API.
// Use this as an escape hatch for endpoints not yet supported by gowa.
// Mirrors pywa's wa.api.send_raw_request().
//
// Parameters:
//   - method:   HTTP verb ("GET", "POST", "DELETE", etc.).
//   - endpoint: Graph API path (e.g. "/{phone_id}/messages") or full URL.
//   - params:   URL query parameters (may be nil).
//   - body:     JSON-serialisable request body (may be nil for GET).
//
// Returns:
//   - map[string]any: decoded JSON response.
//   - error.
//
// Example:
//
//	res, err := wa.SendRawRequest("GET", "/"+wa.PhoneID()+"/phone_numbers", nil, nil)
func (wa *WhatsApp) SendRawRequest(method, endpoint string, params map[string]string, body any) (map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	return wa.api.sendRawRequest(method, endpoint, params, body)
}

// ── Group management ──────────────────────────────────────────────────────────

// CreateGroupOptions holds optional parameters for CreateGroup.
type CreateGroupOptions struct {
	Description      string
	JoinApprovalMode GroupJoinApprovalMode
	PhoneID          string
}

// CreateGroup creates a new WhatsApp group.
// Mirrors pywa's WhatsApp.create_group.
//
// Parameters:
//   - subject: group name/subject (max 128 chars, whitespace trimmed).
//   - opts:    optional CreateGroupOptions.
//
// Returns:
//   - *GroupOperation with the request ID.
//   - error.
func (wa *WhatsApp) CreateGroup(subject string, opts ...CreateGroupOptions) (*GroupOperation, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := CreateGroupOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	pid, err := wa.resolveSender(o.PhoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.createGroup(pid, subject, o.Description, string(o.JoinApprovalMode))
	if err != nil {
		return nil, err
	}
	return &GroupOperation{RequestID: toString(res["request_id"])}, nil
}

// GetGroup fetches details for a single group.
// Mirrors pywa's WhatsApp.get_group.
//
// Parameters:
//   - groupID: the group ID.
//
// Returns:
//   - *GroupDetails.
//   - error.
func (wa *WhatsApp) GetGroup(groupID string) (*GroupDetails, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	fields := "id,subject,description,creation_timestamp,suspended,total_participant_count,participants,join_approval_mode"
	res, err := wa.api.getGroupInfo(groupID, fields)
	if err != nil {
		return nil, err
	}
	return parseGroupDetails(wa, res), nil
}

// GetGroups returns all groups associated with the business phone number.
// Mirrors pywa's WhatsApp.get_groups.
//
// Parameters:
//   - phoneID:    optional phone ID override.
//   - pagination: optional cursor paging.
//
// Returns:
//   - *Result[*GroupDetails].
//   - error.
func (wa *WhatsApp) GetGroups(phoneID string, pagination *Pagination) (*Result[*GroupDetails], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	fields := "id,subject,description,creation_timestamp,suspended,total_participant_count,participants,join_approval_mode"
	pg := map[string]string{}
	if pagination != nil {
		pg = pagination.toDict()
	}
	res, err := wa.api.getActiveGroups(pid, fields, pg)
	if err != nil {
		return nil, err
	}
	r := &Result[*GroupDetails]{}
	for _, d := range toAnySlice(res["data"]) {
		if dm := toMap(d); dm != nil {
			r.Items = append(r.Items, parseGroupDetails(wa, dm))
		}
	}
	if paging := toMap(res["paging"]); paging != nil {
		if cursors := toMap(paging["cursors"]); cursors != nil {
			r.NextCursor = toString(cursors["after"])
		}
	}
	return r, nil
}

// DeleteGroup deletes a group.
// Mirrors pywa's WhatsApp.delete_group.
//
// Parameters:
//   - groupID: the group ID.
//
// Returns:
//   - *GroupOperation with the request ID.
//   - error.
func (wa *WhatsApp) DeleteGroup(groupID string) (*GroupOperation, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.deleteGroup(groupID)
	if err != nil {
		return nil, err
	}
	return &GroupOperation{RequestID: toString(res["request_id"])}, nil
}

// UpdateGroupSettingsOptions holds parameters for UpdateGroupSettings.
type UpdateGroupSettingsOptions struct {
	Subject     string
	Description string
	// ProfilePicture is the raw JPEG bytes for the new group photo (nil = no change).
	ProfilePicture []byte
}

// UpdateGroupSettings updates the subject, description, or profile picture of a group.
// Mirrors pywa's WhatsApp.update_group_settings.
//
// Parameters:
//   - groupID: the group ID.
//   - opts:    fields to update (at least one must be non-zero).
//
// Returns:
//   - *GroupOperation with the request ID.
//   - error.
func (wa *WhatsApp) UpdateGroupSettings(groupID string, opts UpdateGroupSettingsOptions) (*GroupOperation, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.updateGroupInfo(groupID, opts.Subject, opts.Description, opts.ProfilePicture)
	if err != nil {
		return nil, err
	}
	return &GroupOperation{RequestID: toString(res["request_id"])}, nil
}

// GetGroupJoinRequests fetches pending join requests for a group.
// Mirrors pywa's WhatsApp.get_group_join_requests.
//
// Parameters:
//   - groupID:    the group ID.
//   - pagination: optional cursor paging.
//
// Returns:
//   - *Result[*GroupJoinRequest].
//   - error.
func (wa *WhatsApp) GetGroupJoinRequests(groupID string, pagination *Pagination) (*Result[*GroupJoinRequest], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pg := map[string]string{}
	if pagination != nil {
		pg = pagination.toDict()
	}
	res, err := wa.api.getGroupJoinRequests(groupID, pg)
	if err != nil {
		return nil, err
	}
	r := &Result[*GroupJoinRequest]{}
	for _, d := range toAnySlice(res["data"]) {
		dm := toMap(d)
		if dm == nil {
			continue
		}
		r.Items = append(r.Items, parseGroupJoinRequest(wa, groupID, dm))
	}
	if paging := toMap(res["paging"]); paging != nil {
		if cursors := toMap(paging["cursors"]); cursors != nil {
			r.NextCursor = toString(cursors["after"])
		}
	}
	return r, nil
}

// ApproveGroupJoinRequests approves a set of pending join requests.
// Mirrors pywa's WhatsApp.approve_group_join_requests.
//
// Parameters:
//   - groupID:    the group ID.
//   - requestIDs: IDs of join requests to approve.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (wa *WhatsApp) ApproveGroupJoinRequests(groupID string, requestIDs []string) (*GroupOperation, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.approveGroupJoinRequests(groupID, requestIDs)
	if err != nil {
		return nil, err
	}
	return &GroupOperation{RequestID: toString(res["request_id"])}, nil
}

// RejectGroupJoinRequests rejects a set of pending join requests.
// Mirrors pywa's WhatsApp.reject_group_join_requests.
//
// Parameters:
//   - groupID:    the group ID.
//   - requestIDs: IDs of join requests to reject.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (wa *WhatsApp) RejectGroupJoinRequests(groupID string, requestIDs []string) (*GroupOperation, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.rejectGroupJoinRequests(groupID, requestIDs)
	if err != nil {
		return nil, err
	}
	return &GroupOperation{RequestID: toString(res["request_id"])}, nil
}

// GetGroupInviteLink fetches the invite link for a group.
// Mirrors pywa's WhatsApp.get_group_invite_link.
//
// Parameters:
//   - groupID: the group ID.
//
// Returns:
//   - *GroupInviteLink.
//   - error.
func (wa *WhatsApp) GetGroupInviteLink(groupID string) (*GroupInviteLink, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.getGroupInviteLink(groupID)
	if err != nil {
		return nil, err
	}
	return &GroupInviteLink{
		Link:    toString(res["invite_link"]),
		groupID: groupID,
		client:  wa,
	}, nil
}

// ResetGroupInviteLink resets the group's invite link (invalidating the old one).
// Mirrors pywa's WhatsApp.reset_group_invite_link.
//
// Parameters:
//   - groupID: the group ID.
//
// Returns:
//   - *GroupInviteLink with the new link.
//   - error.
func (wa *WhatsApp) ResetGroupInviteLink(groupID string) (*GroupInviteLink, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.resetGroupInviteLink(groupID)
	if err != nil {
		return nil, err
	}
	return &GroupInviteLink{
		Link:    toString(res["invite_link"]),
		groupID: groupID,
		client:  wa,
	}, nil
}

// RemoveGroupParticipants removes participants from a group.
// Mirrors pywa's WhatsApp.remove_group_participants.
//
// Parameters:
//   - groupID:      the group ID.
//   - participants: WA IDs or BSUIDs of participants to remove.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (wa *WhatsApp) RemoveGroupParticipants(groupID string, participants []string) (*GroupOperation, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	res, err := wa.api.removeGroupParticipants(groupID, participants)
	if err != nil {
		return nil, err
	}
	return &GroupOperation{RequestID: toString(res["request_id"])}, nil
}

// ── PinMessage / UnpinMessage ─────────────────────────────────────────────────

// PinMessageOptions holds optional parameters for PinMessage.
type PinMessageOptions struct {
	Sender string
}

// PinMessage pins a message in a chat (currently only supported in group chats).
// Mirrors pywa's WhatsApp.pin_message.
//
// Parameters:
//   - chatID:         the group ID or WA ID of the chat.
//   - messageID:      the wamid of the message to pin.
//   - expirationDays: number of days before the pin expires (1–30).
//   - opts:           optional PinMessageOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) PinMessage(chatID, messageID string, expirationDays int, opts ...PinMessageOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := PinMessageOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}
	payload := sendMessagePayload{
		To:   chatID,
		Type: "pin",
		Interactive: map[string]any{
			"type":            "pin",
			"message_id":      messageID,
			"expiration_days": expirationDays,
		},
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, chatID), nil
}

// UnpinMessage unpins a message in a chat.
// Mirrors pywa's WhatsApp.unpin_message.
//
// Parameters:
//   - chatID:    the group ID or WA ID of the chat.
//   - messageID: the wamid of the message to unpin.
//   - opts:      optional PinMessageOptions.
//
// Returns:
//   - *SentMessage.
//   - error.
func (wa *WhatsApp) UnpinMessage(chatID, messageID string, opts ...PinMessageOptions) (*SentMessage, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := PinMessageOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	sender, err := wa.resolveSender(o.Sender)
	if err != nil {
		return nil, err
	}
	payload := sendMessagePayload{
		To:   chatID,
		Type: "pin",
		Interactive: map[string]any{
			"type":       "unpin",
			"message_id": messageID,
		},
	}
	res, err := wa.api.sendMessage(sender, payload)
	if err != nil {
		return nil, err
	}
	return extractSentMessage(res, sender, chatID), nil
}

// ── Username management ───────────────────────────────────────────────────────

// SetUsernameOptions holds optional parameters for SetUsername.
type SetUsernameOptions struct {
	// ForceTransfer moves an existing username from another number in the same
	// portfolio to this one.
	ForceTransfer bool
	PhoneID       string
}

// SetUsername sets or changes the business username for a phone number.
// Mirrors pywa's WhatsApp.set_username.
//
// Parameters:
//   - username: the desired business username.
//   - opts:     optional SetUsernameOptions.
//
// Returns:
//   - *UsernameStatus with the username and its current approval state.
//   - error.
func (wa *WhatsApp) SetUsername(username string, opts ...SetUsernameOptions) (*UsernameStatus, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	o := SetUsernameOptions{}
	if len(opts) > 0 {
		o = opts[0]
	}
	pid, err := wa.resolveSender(o.PhoneID)
	if err != nil {
		return nil, err
	}
	transferAction := "none"
	if o.ForceTransfer {
		transferAction = "force_transfer"
	}
	res, err := wa.api.setUsername(pid, username, transferAction)
	if err != nil {
		return nil, err
	}
	return &UsernameStatus{
		Username: username,
		Status:   UsernameStatusType(toString(res["status"])),
	}, nil
}

// GetCurrentUsername returns the current username and status for a phone number.
// Mirrors pywa's WhatsApp.get_current_username.
//
// Parameters:
//   - phoneID: optional override.
//
// Returns:
//   - *UsernameStatus.
//   - error.
func (wa *WhatsApp) GetCurrentUsername(phoneID string) (*UsernameStatus, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.getCurrentUsername(pid)
	if err != nil {
		return nil, err
	}
	return &UsernameStatus{
		Username: toString(res["username"]),
		Status:   UsernameStatusType(toString(res["status"])),
	}, nil
}

// GetReservedUsernames returns usernames reserved for the business portfolio.
// These suggestions have a higher chance of approval.
// Mirrors pywa's WhatsApp.get_reserved_usernames.
//
// Parameters:
//   - phoneID: optional override.
//
// Returns:
//   - []string of reserved username suggestions.
//   - error.
func (wa *WhatsApp) GetReservedUsernames(phoneID string) ([]string, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.getReservedUsernames(pid)
	if err != nil {
		return nil, err
	}
	// Response shape: {"data": [{"username_suggestions": ["u1","u2",...]}]}
	data := toAnySlice(res["data"])
	if len(data) == 0 {
		return nil, nil
	}
	first := toMap(data[0])
	return toStringSlice(first["username_suggestions"]), nil
}

// DeleteUsername deletes the business username for a phone number.
// Mirrors pywa's WhatsApp.delete_username.
//
// Parameters:
//   - phoneID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) DeleteUsername(phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.deleteUsername(pid)
	return apiErr
}

// ── Phone number provisioning ─────────────────────────────────────────────────

// CreatePhoneNumber provisions a new phone number on a WABA.
// Mirrors pywa's WhatsApp.create_phone_number.
//
// Parameters:
//   - countryCode:  calling code string (e.g. "1" for US, "234" for Nigeria).
//   - phoneNumber:  number digits.
//   - verifiedName: display name for the phone number.
//   - wabaID:       optional WABA ID override.
//
// Returns:
//   - *CreatedBusinessPhoneNumber with the assigned ID.
//   - error.
func (wa *WhatsApp) CreatePhoneNumber(countryCode, phoneNumber, verifiedName, wabaID string) (*CreatedBusinessPhoneNumber, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.createPhoneNumber(wid, countryCode, phoneNumber, verifiedName)
	if err != nil {
		return nil, err
	}
	return &CreatedBusinessPhoneNumber{ID: toString(res["id"])}, nil
}

// RequestVerificationCode requests an SMS or voice verification code for a
// phone number as part of the Cloud API registration flow.
// Mirrors pywa's WhatsApp.request_verification_code.
//
// Parameters:
//   - codeMethod:   "SMS" | "VOICE".
//   - languageCode: two-character BCP-47 language code (e.g. "en").
//   - phoneID:      optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) RequestVerificationCode(codeMethod, languageCode, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.requestVerificationCode(pid, codeMethod, languageCode)
	return apiErr
}

// VerifyPhoneNumber submits the verification code received by the user to
// complete phone number registration.
// Mirrors pywa's WhatsApp.verify_phone_number.
//
// Parameters:
//   - code:    the verification code string.
//   - phoneID: optional override.
//
// Returns:
//   - error.
func (wa *WhatsApp) VerifyPhoneNumber(code, phoneID string) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return err
	}
	_, apiErr := wa.api.verifyPhoneNumber(pid, code)
	return apiErr
}

// ── WABA portfolio ────────────────────────────────────────────────────────────

// GetSharedBusinessAccounts returns WABAs shared with a business portfolio
// (used by solution providers / ISVs).
// Mirrors pywa's WhatsApp.get_shared_business_accounts.
//
// Parameters:
//   - portfolioID: the business portfolio ID (Meta Business Manager ID).
//   - pagination:  optional cursor paging.
//
// Returns:
//   - *Result[*WhatsAppBusinessAccount].
//   - error.
func (wa *WhatsApp) GetSharedBusinessAccounts(portfolioID string, pagination *Pagination) (*Result[*WhatsAppBusinessAccount], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pg := map[string]string{}
	if pagination != nil {
		pg = pagination.toDict()
	}
	fields := "id,name,currency,message_template_namespace"
	res, err := wa.api.getSharedWABAs(portfolioID, fields, pg)
	if err != nil {
		return nil, err
	}
	return parseWABAResult(res), nil
}

// GetOwnedBusinessAccounts returns WABAs owned by a business portfolio.
// Mirrors pywa's WhatsApp.get_owned_business_accounts.
//
// Parameters:
//   - portfolioID: the business portfolio ID.
//   - pagination:  optional cursor paging.
//
// Returns:
//   - *Result[*WhatsAppBusinessAccount].
//   - error.
func (wa *WhatsApp) GetOwnedBusinessAccounts(portfolioID string, pagination *Pagination) (*Result[*WhatsAppBusinessAccount], error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pg := map[string]string{}
	if pagination != nil {
		pg = pagination.toDict()
	}
	fields := "id,name,currency,message_template_namespace"
	res, err := wa.api.getOwnedWABAs(portfolioID, fields, pg)
	if err != nil {
		return nil, err
	}
	return parseWABAResult(res), nil
}

// GetWABASubscribedApps returns the apps currently subscribed to a WABA's
// webhook events.
// Mirrors pywa's api.get_waba_subscribed_apps.
//
// Parameters:
//   - wabaID: optional WABA ID override.
//
// Returns:
//   - []map[string]any: list of subscribed app objects.
//   - error.
func (wa *WhatsApp) GetWABASubscribedApps(wabaID string) ([]map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.getWABASubscribedApps(wid)
	if err != nil {
		return nil, err
	}
	var apps []map[string]any
	for _, d := range toAnySlice(res["data"]) {
		if dm := toMap(d); dm != nil {
			apps = append(apps, dm)
		}
	}
	return apps, nil
}

// UpdateBusinessAccountSettingsOptions holds parameters for UpdateBusinessAccountSettings.
type UpdateBusinessAccountSettingsOptions struct {
	// DisableMarketingMessagesOnCloudAPI blocks MARKETING templates from being
	// sent via the /messages endpoint.  When true, marketing sends must use
	// the Marketing Messages Lite API.
	DisableMarketingMessagesOnCloudAPI *bool
	// WABAOID overrides the client WABA ID.
	WABAOID string
}

// UpdateBusinessAccountSettings updates settings on a WhatsApp Business Account.
// Mirrors pywa's WhatsApp.update_business_account_settings.
//
// Parameters:
//   - opts: UpdateBusinessAccountSettingsOptions with fields to change.
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateBusinessAccountSettings(opts UpdateBusinessAccountSettingsOptions) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	wid, err := wa.resolveWABAID(opts.WABAOID)
	if err != nil {
		return err
	}
	settings := map[string]any{}
	if opts.DisableMarketingMessagesOnCloudAPI != nil {
		settings["disable_marketing_messages_on_cloud_api"] = *opts.DisableMarketingMessagesOnCloudAPI
	}
	if len(settings) == 0 {
		return fmt.Errorf("gowa: UpdateBusinessAccountSettings: at least one setting must be provided")
	}
	_, apiErr := wa.api.updateWABASettings(wid, settings)
	return apiErr
}

// ── Template archive ──────────────────────────────────────────────────────────

// ArchiveTemplates archives up to 100 templates at a time.
// Archived templates can no longer be sent but are preserved for compliance.
// Mirrors pywa's WhatsApp.archive_templates.
//
// Parameters:
//   - templateIDs: IDs of templates to archive (max 100).
//   - wabaID:      optional WABA ID override.
//
// Returns:
//   - *ArchiveTemplatesResult.
//   - error.
func (wa *WhatsApp) ArchiveTemplates(templateIDs []string, wabaID string) (*ArchiveTemplatesResult, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.archiveTemplates(wid, templateIDs)
	if err != nil {
		return nil, err
	}
	return parseArchiveResult(res), nil
}

// UnarchiveTemplates restores up to 100 archived templates.
// Mirrors pywa's WhatsApp.unarchive_templates.
//
// Parameters:
//   - templateIDs: IDs of templates to unarchive (max 100).
//   - wabaID:      optional WABA ID override.
//
// Returns:
//   - *UnarchiveTemplatesResult.
//   - error.
func (wa *WhatsApp) UnarchiveTemplates(templateIDs []string, wabaID string) (*UnarchiveTemplatesResult, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	wid, err := wa.resolveWABAID(wabaID)
	if err != nil {
		return nil, err
	}
	res, err := wa.api.unarchiveTemplates(wid, templateIDs)
	if err != nil {
		return nil, err
	}
	return parseUnarchiveResult(res), nil
}

// ── Parse helpers ─────────────────────────────────────────────────────────────

// parseGroupDetails converts a raw API map into a *GroupDetails.
func parseGroupDetails(wa *WhatsApp, m map[string]any) *GroupDetails {
	g := &GroupDetails{
		ID:                    toString(m["id"]),
		Subject:               toString(m["subject"]),
		Description:           toString(m["description"]),
		CreationTimestamp:     toTime(m["creation_timestamp"]),
		Suspended:             toBool(m["suspended"]),
		TotalParticipantCount: int(toFloat64(m["total_participant_count"])),
		JoinApprovalMode:      GroupJoinApprovalMode(toString(m["join_approval_mode"])),
		client:                wa,
	}
	for _, p := range toAnySlice(m["participants"]) {
		pm := toMap(p)
		if pm == nil {
			continue
		}
		g.Participants = append(g.Participants, GroupParticipant{
			BSUID:       toString(pm["user_id"]),
			WAID:        toString(pm["wa_id"]),
			Username:    toString(pm["username"]),
			ParentBSUID: toString(pm["parent_user_id"]),
			groupID:     g.ID,
			client:      wa,
		})
	}
	return g
}

// parseGroupJoinRequest converts a raw API map into a *GroupJoinRequest.
func parseGroupJoinRequest(wa *WhatsApp, groupID string, m map[string]any) *GroupJoinRequest {
	return &GroupJoinRequest{
		ID: toString(m["join_request_id"]),
		User: GroupParticipant{
			BSUID:   toString(m["user_id"]),
			WAID:    toString(m["wa_id"]),
			groupID: groupID,
			client:  wa,
		},
		CreationTimestamp: toTime(m["creation_timestamp"]),
		groupID:           groupID,
		client:            wa,
	}
}

// parseWABAResult decodes a paginated WABA list response.
func parseWABAResult(res map[string]any) *Result[*WhatsAppBusinessAccount] {
	r := &Result[*WhatsAppBusinessAccount]{}
	for _, d := range toAnySlice(res["data"]) {
		dm := toMap(d)
		if dm == nil {
			continue
		}
		r.Items = append(r.Items, &WhatsAppBusinessAccount{
			ID:                       toString(dm["id"]),
			Name:                     toString(dm["name"]),
			Currency:                 toString(dm["currency"]),
			MessageTemplateNamespace: toString(dm["message_template_namespace"]),
		})
	}
	if paging := toMap(res["paging"]); paging != nil {
		if cursors := toMap(paging["cursors"]); cursors != nil {
			r.NextCursor = toString(cursors["after"])
		}
	}
	return r
}

// parseArchiveResult decodes an archive templates response.
func parseArchiveResult(res map[string]any) *ArchiveTemplatesResult {
	r := &ArchiveTemplatesResult{}
	for _, d := range toAnySlice(res["archived_templates"]) {
		dm := toMap(d)
		if dm != nil {
			r.ArchivedTemplates = append(r.ArchivedTemplates, TemplateArchiveEntry{
				ID: toString(dm["id"]), Name: toString(dm["name"]),
			})
		}
	}
	for _, d := range toAnySlice(res["failed_templates"]) {
		dm := toMap(d)
		if dm != nil {
			r.FailedTemplates = append(r.FailedTemplates, TemplateArchiveEntry{
				ID: toString(dm["id"]), Name: toString(dm["name"]),
			})
		}
	}
	return r
}

// parseUnarchiveResult decodes an unarchive templates response.
func parseUnarchiveResult(res map[string]any) *UnarchiveTemplatesResult {
	r := &UnarchiveTemplatesResult{}
	for _, d := range toAnySlice(res["unarchived_templates"]) {
		dm := toMap(d)
		if dm != nil {
			r.UnarchivedTemplates = append(r.UnarchivedTemplates, TemplateArchiveEntry{
				ID: toString(dm["id"]), Name: toString(dm["name"]),
			})
		}
	}
	for _, d := range toAnySlice(res["failed_templates"]) {
		dm := toMap(d)
		if dm != nil {
			r.FailedTemplates = append(r.FailedTemplates, TemplateArchiveEntry{
				ID: toString(dm["id"]), Name: toString(dm["name"]),
			})
		}
	}
	return r
}
