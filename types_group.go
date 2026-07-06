package gowa

import "time"

// ────────────────────────────────────────────────────────────────────────────────
// Group types
// Mirrors pywa/types/groups.py — introduced in pywa 4.1.0 / 4.2.0.
// ────────────────────────────────────────────────────────────────────────────────

// GroupJoinApprovalMode controls whether users clicking the invite link can join
// immediately or must wait for approval.
// Mirrors pywa's GroupJoinApprovalMode StrEnum.
type GroupJoinApprovalMode string

const (
	// GroupJoinAutoApprove lets users join without any approval step.
	GroupJoinAutoApprove GroupJoinApprovalMode = "auto_approve"
	// GroupJoinApprovalRequired means users must be approved before joining.
	GroupJoinApprovalRequired GroupJoinApprovalMode = "approval_required"
)

// GroupOperation is returned by mutating group API calls (create, delete,
// remove participants, etc.).  The RequestID can be used to track the
// asynchronous status of the operation.
// Mirrors pywa's GroupOperation dataclass.
type GroupOperation struct {
	// RequestID is the unique identifier for this operation.
	RequestID string
}

// GroupParticipant represents a member of a WhatsApp group.
// Mirrors pywa's GroupParticipant dataclass.
//
// Fields:
//   - BSUID:        Business-scoped user ID.
//   - WAID:         Phone number in E.164 (without '+'). Empty when the user
//     has enabled the username feature.
//   - Username:     Business-facing username (empty when not set).
//   - ParentBSUID:  Parent business-scoped user ID (optional).
type GroupParticipant struct {
	BSUID       string
	WAID        string
	Username    string
	ParentBSUID string

	groupID string
	client  *WhatsApp
}

// Remove removes this participant from the group.
// Shortcut for WhatsApp.RemoveGroupParticipants.
//
// Returns:
//   - *GroupOperation with the request ID.
//   - error if the API call fails.
func (p *GroupParticipant) Remove() (*GroupOperation, error) {
	if p.client == nil {
		return nil, ErrNoClient
	}
	return p.client.RemoveGroupParticipants(p.groupID, []string{p.preferredID()})
}

// preferredID returns BSUID when set, otherwise WAID.
func (p *GroupParticipant) preferredID() string {
	if p.BSUID != "" {
		return p.BSUID
	}
	return p.WAID
}

// GroupDetails holds the full metadata for a WhatsApp group.
// Mirrors pywa's GroupDetails dataclass.
//
// Fields:
//   - ID:                    Group ID.
//   - Subject:               Group subject / name.
//   - Description:           Group description (may be empty).
//   - CreationTimestamp:     When the group was created (UTC).
//   - Suspended:             True if the group has been suspended by WhatsApp.
//   - TotalParticipantCount: Number of participants (excluding the business).
//   - Participants:          List of participants (excluding the business).
//   - JoinApprovalMode:      Whether joining requires approval.
type GroupDetails struct {
	ID                    string
	Subject               string
	Description           string
	CreationTimestamp     time.Time
	Suspended             bool
	TotalParticipantCount int
	Participants          []GroupParticipant
	JoinApprovalMode      GroupJoinApprovalMode

	client *WhatsApp
}

// GetInviteLink fetches the invite link for the group.
// Shortcut for WhatsApp.GetGroupInviteLink.
//
// Returns:
//   - *GroupInviteLink.
//   - error.
func (g *GroupDetails) GetInviteLink() (*GroupInviteLink, error) {
	if g.client == nil {
		return nil, ErrNoClient
	}
	return g.client.GetGroupInviteLink(g.ID)
}

// ResetInviteLink resets the invite link for the group.
// Shortcut for WhatsApp.ResetGroupInviteLink.
//
// Returns:
//   - *GroupInviteLink with the new link.
//   - error.
func (g *GroupDetails) ResetInviteLink() (*GroupInviteLink, error) {
	if g.client == nil {
		return nil, ErrNoClient
	}
	return g.client.ResetGroupInviteLink(g.ID)
}

// GetJoinRequests fetches pending join requests for the group.
// Shortcut for WhatsApp.GetGroupJoinRequests.
//
// Returns:
//   - *Result[*GroupJoinRequest].
//   - error.
func (g *GroupDetails) GetJoinRequests(pagination *Pagination) (*Result[*GroupJoinRequest], error) {
	if g.client == nil {
		return nil, ErrNoClient
	}
	return g.client.GetGroupJoinRequests(g.ID, pagination)
}

// Delete deletes the group.
// Shortcut for WhatsApp.DeleteGroup.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (g *GroupDetails) Delete() (*GroupOperation, error) {
	if g.client == nil {
		return nil, ErrNoClient
	}
	return g.client.DeleteGroup(g.ID)
}

// RemoveParticipants removes a set of participants from the group.
// Shortcut for WhatsApp.RemoveGroupParticipants.
//
// Parameters:
//   - participants: WA IDs or BSUIDs of participants to remove.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (g *GroupDetails) RemoveParticipants(participants []string) (*GroupOperation, error) {
	if g.client == nil {
		return nil, ErrNoClient
	}
	return g.client.RemoveGroupParticipants(g.ID, participants)
}

// RemoveAllParticipants removes every participant from the group.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (g *GroupDetails) RemoveAllParticipants() (*GroupOperation, error) {
	if g.client == nil {
		return nil, ErrNoClient
	}
	ids := make([]string, len(g.Participants))
	for i, p := range g.Participants {
		ids[i] = p.preferredID()
	}
	return g.client.RemoveGroupParticipants(g.ID, ids)
}

// GroupInviteLink holds the invite link for a group.
// Mirrors pywa's GroupInviteLink dataclass.
type GroupInviteLink struct {
	// Link is the full invitation URL.
	Link    string
	groupID string
	client  *WhatsApp
}

// Reset resets the invite link (the old link becomes invalid).
// Shortcut for WhatsApp.ResetGroupInviteLink.
//
// Returns:
//   - *GroupInviteLink with the new link.
//   - error.
func (l *GroupInviteLink) Reset() (*GroupInviteLink, error) {
	if l.client == nil {
		return nil, ErrNoClient
	}
	return l.client.ResetGroupInviteLink(l.groupID)
}

// GroupJoinRequest represents a pending join request for a group.
// Mirrors pywa's GroupJoinRequest dataclass.
//
// Fields:
//   - ID:                  The join request ID.
//   - User:                The participant who submitted the request.
//   - CreationTimestamp:   When the request was submitted (UTC).
type GroupJoinRequest struct {
	ID                string
	User              GroupParticipant
	CreationTimestamp time.Time

	groupID string
	client  *WhatsApp
}

// Approve approves this join request.
// Shortcut for WhatsApp.ApproveGroupJoinRequests.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (r *GroupJoinRequest) Approve() (*GroupOperation, error) {
	if r.client == nil {
		return nil, ErrNoClient
	}
	return r.client.ApproveGroupJoinRequests(r.groupID, []string{r.ID})
}

// Reject rejects this join request.
// Shortcut for WhatsApp.RejectGroupJoinRequests.
//
// Returns:
//   - *GroupOperation.
//   - error.
func (r *GroupJoinRequest) Reject() (*GroupOperation, error) {
	if r.client == nil {
		return nil, ErrNoClient
	}
	return r.client.RejectGroupJoinRequests(r.groupID, []string{r.ID})
}

// ── Username types ────────────────────────────────────────────────────────────

// UsernameStatusType is the approval state of a business username.
// Mirrors pywa's UsernameStatusType.
type UsernameStatusType string

const (
	UsernameStatusAvailable   UsernameStatusType = "AVAILABLE"
	UsernameStatusPending     UsernameStatusType = "PENDING"
	UsernameStatusApproved    UsernameStatusType = "APPROVED"
	UsernameStatusRejected    UsernameStatusType = "REJECTED"
	UsernameStatusUnavailable UsernameStatusType = "UNAVAILABLE"
)

// UsernameStatus holds a business username and its current status.
// Mirrors pywa's UsernameStatus dataclass.
type UsernameStatus struct {
	// Username is the business username string.
	Username string
	// Status is the current review/availability status.
	Status UsernameStatusType
}

// ── Carousel card types ───────────────────────────────────────────────────────

// CarouselCard is the interface implemented by all carousel card variants.
// Use ImageCarouselCard or VideoCarouselCard to build carousel messages.
// Mirrors pywa's BaseCarouselCard.
type CarouselCard interface {
	toCarouselDict(idx int) map[string]any
}

// CarouselButtons is either a []Button (quick-reply) or a *URLButton (CTA).
// Only one type is valid per card — the same constraint as on regular messages.
type CarouselButtons interface{}

// ImageCarouselCard is a carousel card with an image header.
// Mirrors pywa's ImageCarouselCard dataclass.
//
// Fields:
//   - Image:   Publicly accessible HTTPS image URL (JPEG or PNG).
//   - Body:    Optional card body text (max 160 chars, up to 2 line breaks).
//   - Buttons: []Button (max 3 quick-reply) or *URLButton (1 CTA).
type ImageCarouselCard struct {
	Image   string
	Body    string
	Buttons CarouselButtons
}

func (c ImageCarouselCard) toCarouselDict(idx int) map[string]any {
	d := buildCarouselBase(idx, c.Body, c.Buttons)
	d["header"] = map[string]any{
		"type":  "image",
		"image": map[string]any{"link": c.Image},
	}
	return d
}

// VideoCarouselCard is a carousel card with a video header.
// Mirrors pywa's VideoCarouselCard dataclass.
//
// Fields:
//   - Video:   Publicly accessible HTTPS video URL (MP4).
//   - Body:    Optional card body text (max 160 chars, up to 2 line breaks).
//   - Buttons: []Button (max 3 quick-reply) or *URLButton (1 CTA).
type VideoCarouselCard struct {
	Video   string
	Body    string
	Buttons CarouselButtons
}

func (c VideoCarouselCard) toCarouselDict(idx int) map[string]any {
	d := buildCarouselBase(idx, c.Body, c.Buttons)
	d["header"] = map[string]any{
		"type":  "video",
		"video": map[string]any{"link": c.Video},
	}
	return d
}

// buildCarouselBase builds the common portion of a carousel card dict.
func buildCarouselBase(idx int, body string, buttons CarouselButtons) map[string]any {
	d := map[string]any{"card_index": idx}
	if body != "" {
		d["body"] = map[string]any{"text": body}
	}
	switch b := buttons.(type) {
	case []Button:
		btnMaps := make([]map[string]any, len(b))
		for i, btn := range b {
			btnMaps[i] = map[string]any{
				"type":        "quick_reply",
				"quick_reply": map[string]any{"id": btn.ID, "title": btn.Title},
			}
		}
		d["action"] = map[string]any{"buttons": btnMaps}
	case *URLButton:
		d["action"] = map[string]any{
			"name": "cta_url",
			"parameters": map[string]any{
				"display_text": b.Title,
				"url":          b.URL,
			},
		}
	}
	return d
}

// ── Template archive result types ─────────────────────────────────────────────

// TemplateArchiveEntry is a single entry in an archive/unarchive result.
type TemplateArchiveEntry struct {
	ID   string
	Name string
}

// ArchiveTemplatesResult is returned by ArchiveTemplates.
// Mirrors pywa's ArchiveTemplatesResult.
type ArchiveTemplatesResult struct {
	ArchivedTemplates []TemplateArchiveEntry
	FailedTemplates   []TemplateArchiveEntry
}

// UnarchiveTemplatesResult is returned by UnarchiveTemplates.
// Mirrors pywa's UnarchiveTemplatesResult.
type UnarchiveTemplatesResult struct {
	UnarchivedTemplates []TemplateArchiveEntry
	FailedTemplates     []TemplateArchiveEntry
}

// ── Phone number provisioning ─────────────────────────────────────────────────

// CreatedBusinessPhoneNumber holds the ID of a newly provisioned phone number.
// Mirrors pywa's CreatedBusinessPhoneNumber dataclass.
type CreatedBusinessPhoneNumber struct {
	// ID is the phone number ID assigned by Meta.
	ID string
}

// ── WABA portfolio ────────────────────────────────────────────────────────────

// WABAPortfolioResult is a paginated list of WhatsAppBusinessAccount objects.
// Returned by GetSharedBusinessAccounts and GetOwnedBusinessAccounts.
type WABAPortfolioResult = Result[*WhatsAppBusinessAccount]
