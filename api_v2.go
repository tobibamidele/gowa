package gowa

import (
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// textprotoMIMEHeader is a local alias so api_v2.go can use textproto.MIMEHeader
// without confusion — the canonical import is already in api.go (same package).
type textprotoMIMEHeader = textproto.MIMEHeader

// multipartWriterAlias keeps the compiler happy when api_v2.go references
// *multipart.Writer via the multipartWriter alias declared in client_extended.go.
var _ *multipart.Writer // ensure import is used

// ────────────────────────────────────────────────────────────────────────────────
// api_v2.go — new low-level Graph API methods added in pywa 4.1–4.2.
// All methods follow the same contract as api.go: they return
// (map[string]any, error) and are called only from client_v2.go.
// ────────────────────────────────────────────────────────────────────────────────

// ── Groups ────────────────────────────────────────────────────────────────────

// createGroup creates a new WhatsApp group.
//
// Parameters:
//   - phoneID:          phone number ID.
//   - subject:          group name/subject (max 128 chars).
//   - description:      optional group description (max 2048 chars).
//   - joinApprovalMode: "auto_approve" | "approval_required" | "" (default).
//
// Returns:
//   - map with "request_id".
//   - error.
func (g *graphAPI) createGroup(phoneID, subject, description, joinApprovalMode string) (map[string]any, error) {
	body := map[string]any{
		"messaging_product": "whatsapp",
		"subject":           subject,
	}
	if description != "" {
		body["description"] = description
	}
	if joinApprovalMode != "" {
		body["join_approval_mode"] = joinApprovalMode
	}
	return g.post("/"+phoneID+"/groups", body)
}

// getGroupInfo fetches metadata for a single group.
//
// Parameters:
//   - groupID: group ID.
//   - fields:  comma-separated field list (empty = server default).
//
// Returns:
//   - map with group data.
//   - error.
func (g *graphAPI) getGroupInfo(groupID, fields string) (map[string]any, error) {
	params := map[string]string{}
	if fields != "" {
		params["fields"] = fields
	}
	return g.get("/"+groupID, params)
}

// getActiveGroups returns all groups associated with a phone number.
//
// Parameters:
//   - phoneID:    phone number ID.
//   - fields:     optional field selector.
//   - pagination: optional cursor params.
//
// Returns:
//   - map with "data" array and "paging".
//   - error.
func (g *graphAPI) getActiveGroups(phoneID, fields string, pagination map[string]string) (map[string]any, error) {
	params := map[string]string{}
	for k, v := range pagination {
		params[k] = v
	}
	if fields != "" {
		params["fields"] = fields
	}
	return g.get("/"+phoneID+"/groups", params)
}

// deleteGroup deletes a group.
//
// Parameters:
//   - groupID: group ID.
//
// Returns:
//   - map with "request_id".
//   - error.
func (g *graphAPI) deleteGroup(groupID string) (map[string]any, error) {
	return g.delete("/"+groupID, nil)
}

// updateGroupInfo updates a group's subject, description, or profile picture.
// Only non-empty fields are sent.
//
// Parameters:
//   - groupID:     group ID.
//   - subject:     new subject (empty = no change).
//   - description: new description (empty = no change).
//   - profilePic:  raw image bytes (nil = no change).
//
// Returns:
//   - map with "request_id".
//   - error.
func (g *graphAPI) updateGroupInfo(groupID, subject, description string, profilePic []byte) (map[string]any, error) {
	if profilePic != nil {
		// Profile picture must be sent as multipart/form-data.
		return g.request("POST", "/"+groupID, nil, nil, func(mw *multipartWriter) error {
			if err := mw.WriteField("messaging_product", "whatsapp"); err != nil {
				return err
			}
			if subject != "" {
				if err := mw.WriteField("subject", subject); err != nil {
					return err
				}
			}
			if description != "" {
				if err := mw.WriteField("description", description); err != nil {
					return err
				}
			}
			h := make(textprotoMIMEHeader)
			h.Set("Content-Disposition", `form-data; name="profile_picture_file"; filename="profile.jpg"`)
			h.Set("Content-Type", "image/jpeg")
			part, err := mw.CreatePart(h)
			if err != nil {
				return err
			}
			_, err = part.Write(profilePic)
			return err
		})
	}
	body := map[string]any{"messaging_product": "whatsapp"}
	if subject != "" {
		body["subject"] = subject
	}
	if description != "" {
		body["description"] = description
	}
	return g.post("/"+groupID, body)
}

// getGroupJoinRequests returns pending join requests for a group.
//
// Parameters:
//   - groupID:    group ID.
//   - pagination: optional cursor params.
//
// Returns:
//   - map with "data" array.
//   - error.
func (g *graphAPI) getGroupJoinRequests(groupID string, pagination map[string]string) (map[string]any, error) {
	params := map[string]string{}
	for k, v := range pagination {
		params[k] = v
	}
	return g.get("/"+groupID+"/join_requests", params)
}

// approveGroupJoinRequests approves a set of join requests.
//
// Parameters:
//   - groupID:    group ID.
//   - requestIDs: IDs of the join requests to approve.
//
// Returns:
//   - map with "request_id".
//   - error.
func (g *graphAPI) approveGroupJoinRequests(groupID string, requestIDs []string) (map[string]any, error) {
	return g.post("/"+groupID+"/join_requests", map[string]any{
		"messaging_product": "whatsapp",
		"join_requests":     requestIDs,
	})
}

// rejectGroupJoinRequests rejects a set of join requests.
//
// Parameters:
//   - groupID:    group ID.
//   - requestIDs: IDs of the join requests to reject.
//
// Returns:
//   - map with "request_id".
//   - error.
func (g *graphAPI) rejectGroupJoinRequests(groupID string, requestIDs []string) (map[string]any, error) {
	return g.request("DELETE", "/"+groupID+"/join_requests", nil,
		map[string]any{
			"messaging_product": "whatsapp",
			"join_requests":     requestIDs,
		}, nil)
}

// getGroupInviteLink returns the invite link for a group.
//
// Parameters:
//   - groupID: group ID.
//
// Returns:
//   - map with "invite_link".
//   - error.
func (g *graphAPI) getGroupInviteLink(groupID string) (map[string]any, error) {
	return g.get("/"+groupID+"/invite_link", nil)
}

// resetGroupInviteLink resets the invite link, invalidating the old one.
//
// Parameters:
//   - groupID: group ID.
//
// Returns:
//   - map with "invite_link".
//   - error.
func (g *graphAPI) resetGroupInviteLink(groupID string) (map[string]any, error) {
	return g.post("/"+groupID+"/invite_link", map[string]any{"messaging_product": "whatsapp"})
}

// removeGroupParticipants removes participants from a group.
//
// Parameters:
//   - groupID:      group ID.
//   - participants: WA IDs of participants to remove.
//
// Returns:
//   - map with "request_id".
//   - error.
func (g *graphAPI) removeGroupParticipants(groupID string, participants []string) (map[string]any, error) {
	items := make([]map[string]any, len(participants))
	for i, p := range participants {
		items[i] = map[string]any{"user": p}
	}
	return g.request("DELETE", "/"+groupID+"/participants", nil,
		map[string]any{
			"messaging_product": "whatsapp",
			"participants":      items,
		}, nil)
}

// ── Username ──────────────────────────────────────────────────────────────────

// setUsername sets or changes a business username.
//
// Parameters:
//   - phoneID:        phone number ID.
//   - username:       the desired username.
//   - transferAction: "none" | "force_transfer" | "".
//
// Returns:
//   - map with "status".
//   - error.
func (g *graphAPI) setUsername(phoneID, username, transferAction string) (map[string]any, error) {
	body := map[string]any{"username": username}
	if transferAction != "" {
		body["transfer_action"] = transferAction
	}
	return g.post("/"+phoneID+"/username", body)
}

// getCurrentUsername fetches the current username and its status.
//
// Parameters:
//   - phoneID: phone number ID.
//
// Returns:
//   - map with "username" and "status".
//   - error.
func (g *graphAPI) getCurrentUsername(phoneID string) (map[string]any, error) {
	return g.get("/"+phoneID+"/username", nil)
}

// getReservedUsernames fetches username suggestions reserved for the portfolio.
//
// Parameters:
//   - phoneID: phone number ID.
//
// Returns:
//   - map with "data" array of suggestions.
//   - error.
func (g *graphAPI) getReservedUsernames(phoneID string) (map[string]any, error) {
	return g.get("/"+phoneID+"/username_suggestions", nil)
}

// deleteUsername deletes the business username for a phone number.
//
// Parameters:
//   - phoneID: phone number ID.
//
// Returns:
//   - map with "success".
//   - error.
func (g *graphAPI) deleteUsername(phoneID string) (map[string]any, error) {
	return g.delete("/"+phoneID+"/username", nil)
}

// ── Phone number provisioning ─────────────────────────────────────────────────

// createPhoneNumber provisions a new phone number on a WABA.
//
// Parameters:
//   - wabaID:       WABA ID.
//   - countryCode:  calling code (e.g. "1" for US).
//   - phoneNumber:  number digits.
//   - verifiedName: display name for the number.
//
// Returns:
//   - map with "id".
//   - error.
func (g *graphAPI) createPhoneNumber(wabaID, countryCode, phoneNumber, verifiedName string) (map[string]any, error) {
	return g.post("/"+wabaID+"/phone_numbers", map[string]any{
		"country_code":  countryCode,
		"phone_number":  phoneNumber,
		"verified_name": verifiedName,
	})
}

// requestVerificationCode requests a verification code for a phone number.
//
// Parameters:
//   - phoneID:     phone number ID.
//   - codeMethod:  "SMS" | "VOICE".
//   - language:    two-letter language code (e.g. "en").
//
// Returns:
//   - map with "success".
//   - error.
func (g *graphAPI) requestVerificationCode(phoneID, codeMethod, language string) (map[string]any, error) {
	return g.request("POST", "/"+phoneID+"/request_code",
		map[string]string{"code_method": codeMethod, "language": language},
		nil, nil)
}

// verifyPhoneNumber verifies a phone number with the received code.
//
// Parameters:
//   - phoneID: phone number ID.
//   - code:    the verification code.
//
// Returns:
//   - map with "success".
//   - error.
func (g *graphAPI) verifyPhoneNumber(phoneID, code string) (map[string]any, error) {
	return g.post("/"+phoneID+"/verify_code", map[string]any{"code": code})
}

// ── WABA portfolio ────────────────────────────────────────────────────────────

// getSharedWABAs returns WABAs shared with a business portfolio.
//
// Parameters:
//   - portfolioID: business portfolio ID.
//   - fields:      optional comma-separated field list.
//   - pagination:  optional cursor params.
//
// Returns:
//   - map with "data" and "paging".
//   - error.
func (g *graphAPI) getSharedWABAs(portfolioID, fields string, pagination map[string]string) (map[string]any, error) {
	params := map[string]string{}
	for k, v := range pagination {
		params[k] = v
	}
	if fields != "" {
		params["fields"] = fields
	}
	return g.get("/"+portfolioID+"/client_whatsapp_business_accounts", params)
}

// getOwnedWABAs returns WABAs owned by a business portfolio.
//
// Parameters:
//   - portfolioID: business portfolio ID.
//   - fields:      optional comma-separated field list.
//   - pagination:  optional cursor params.
//
// Returns:
//   - map with "data" and "paging".
//   - error.
func (g *graphAPI) getOwnedWABAs(portfolioID, fields string, pagination map[string]string) (map[string]any, error) {
	params := map[string]string{}
	for k, v := range pagination {
		params[k] = v
	}
	if fields != "" {
		params["fields"] = fields
	}
	return g.get("/"+portfolioID+"/owned_whatsapp_business_accounts", params)
}

// getWABASubscribedApps returns apps subscribed to a WABA's webhooks.
//
// Parameters:
//   - wabaID: WABA ID.
//
// Returns:
//   - map with "data" list of subscribed apps.
//   - error.
func (g *graphAPI) getWABASubscribedApps(wabaID string) (map[string]any, error) {
	return g.get("/"+wabaID+"/subscribed_apps", nil)
}

// updateWABASettings updates settings on a WhatsApp Business Account.
//
// Parameters:
//   - wabaID:    WABA ID.
//   - settings:  map of settings to update.
//
// Returns:
//   - map with "id".
//   - error.
func (g *graphAPI) updateWABASettings(wabaID string, settings map[string]any) (map[string]any, error) {
	return g.post("/"+wabaID, settings)
}

// ── Template archive ──────────────────────────────────────────────────────────

// archiveTemplates archives a batch of templates (up to 100).
// Note: this endpoint uses api.facebook.com, not graph.facebook.com.
//
// Parameters:
//   - wabaID:       WABA ID.
//   - templateIDs:  IDs of templates to archive.
//
// Returns:
//   - map with "archived_templates" and "failed_templates".
//   - error.
func (g *graphAPI) archiveTemplates(wabaID string, templateIDs []string) (map[string]any, error) {
	url := fmt.Sprintf("https://api.facebook.com/v%s/%s/message_templates/archive", g.version, wabaID)
	return g.request("POST", url, nil, map[string]any{
		"hsm_ids": strings.Join(templateIDs, ","),
	}, nil)
}

// unarchiveTemplates unarchives a batch of templates (up to 100).
//
// Parameters:
//   - wabaID:       WABA ID.
//   - templateIDs:  IDs of templates to unarchive.
//
// Returns:
//   - map with "unarchived_templates" and "failed_templates".
//   - error.
func (g *graphAPI) unarchiveTemplates(wabaID string, templateIDs []string) (map[string]any, error) {
	url := fmt.Sprintf("https://api.facebook.com/v%s/%s/message_templates/unarchive", g.version, wabaID)
	return g.request("POST", url, nil, map[string]any{
		"hsm_ids": strings.Join(templateIDs, ","),
	}, nil)
}

// ── Raw request ───────────────────────────────────────────────────────────────

// sendRawRequest sends an arbitrary request to the Graph API.
// Use this as an escape hatch for endpoints not yet supported by gowa.
//
// Parameters:
//   - method:   HTTP verb (GET, POST, DELETE, etc.).
//   - endpoint: path or full URL (e.g. "/{phone_id}/messages").
//   - params:   URL query parameters (may be nil).
//   - body:     JSON-serialisable body (may be nil).
//
// Returns:
//   - map[string]any: decoded JSON response.
//   - error.
func (g *graphAPI) sendRawRequest(method, endpoint string, params map[string]string, body any) (map[string]any, error) {
	return g.request(method, endpoint, params, body, nil)
}
