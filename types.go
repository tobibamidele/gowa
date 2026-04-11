package gowa

// ────────────────────────────────────────────────────────────────────────────────
// Core value types
// Mirrors pywa/types/others.py, media.py, callback.py, message.py, etc.
// ────────────────────────────────────────────────────────────────────────────────

import (
	"fmt"
	"time"
)

// ── User ──────────────────────────────────────────────────────────────────────

// User represents a WhatsApp user (sender or contact in a message).
// Maps to pywa's types.User dataclass.
//
// Fields:
//   - WAID:            WhatsApp ID (E.164 phone number without '+').
//   - Name:            Display name supplied by WhatsApp (empty in MessageStatus).
//   - IdentityKeyHash: Optional identity key hash when the feature is enabled.
type User struct {
	WAID            string
	Name            string
	IdentityKeyHash string

	client *WhatsApp // back-reference for shortcut methods
}

// String returns a human-readable representation.
func (u User) String() string {
	return fmt.Sprintf("User(wa_id=%s, name=%s)", u.WAID, u.Name)
}

// Block blocks the user from messaging the business.
// Shortcut for WhatsApp.BlockUsers([u.WAID]).
//
// Returns:
//   - error if the block operation fails.
func (u *User) Block() error {
	if u.client == nil {
		return fmt.Errorf("user is not associated with a client")
	}
	_, err := u.client.BlockUsers([]string{u.WAID})
	return err
}

// Unblock unblocks the user.
// Shortcut for WhatsApp.UnblockUsers([u.WAID]).
//
// Returns:
//   - error if the unblock operation fails.
func (u *User) Unblock() error {
	if u.client == nil {
		return fmt.Errorf("user is not associated with a client")
	}
	_, err := u.client.UnblockUsers([]string{u.WAID})
	return err
}

// ── MessageType ───────────────────────────────────────────────────────────────

// MessageType enumerates the types of incoming WhatsApp messages.
// Maps to pywa's MessageType StrEnum.
type MessageType string

const (
	MessageTypeText           MessageType = "text"
	MessageTypeImage          MessageType = "image"
	MessageTypeVideo          MessageType = "video"
	MessageTypeDocument       MessageType = "document"
	MessageTypeAudio          MessageType = "audio"
	MessageTypeSticker        MessageType = "sticker"
	MessageTypeReaction       MessageType = "reaction"
	MessageTypeLocation       MessageType = "location"
	MessageTypeContacts       MessageType = "contacts"
	MessageTypeOrder          MessageType = "order"
	MessageTypeInteractive    MessageType = "interactive"
	MessageTypeButton         MessageType = "button"
	MessageTypeSystem         MessageType = "system"
	MessageTypeRequestWelcome MessageType = "request_welcome"
	MessageTypeUnknown        MessageType = "unknown"
	MessageTypeUnsupported    MessageType = "unsupported"
)

// ── Metadata ──────────────────────────────────────────────────────────────────

// Metadata holds the phone number details of the receiving business number.
// Maps to pywa's Metadata dataclass.
type Metadata struct {
	DisplayPhoneNumber string
	PhoneNumberID      string
}

// ── Location ──────────────────────────────────────────────────────────────────

// Location represents a geographic location in a message.
// Maps to pywa's Location dataclass.
type Location struct {
	Latitude  float64
	Longitude float64
	Name      string
	Address   string
	URL       string
}

// ── Reaction ──────────────────────────────────────────────────────────────────

// Reaction holds an emoji reaction to a message.
// Maps to pywa's Reaction dataclass.
type Reaction struct {
	MessageID string
	Emoji     string
}

// ── ReplyToMessage ─────────────────────────────────────────────────────────────

// ReplyToMessage contains context about the message being replied to.
// Maps to pywa's ReplyToMessage dataclass.
type ReplyToMessage struct {
	MessageID string
	FromWAID  string
}

// ── Referral ──────────────────────────────────────────────────────────────────

// Referral holds click-to-WhatsApp ad information included when a user opens a
// conversation via an ad.
// Maps to pywa's Referral dataclass.
type Referral struct {
	SourceURL  string
	SourceID   string
	SourceType string
	Headline   string
	Body       string
	MediaType  string
	ImageURL   string
	VideoURL   string
	CTWAClid   string
}

// ── Order ─────────────────────────────────────────────────────────────────────

// OrderItem represents a single product in a WhatsApp order.
type OrderItem struct {
	ProductRetailerID string
	Quantity          int
	ItemPrice         float64
	Currency          string
}

// Order is sent when a user places an order from a catalog.
// Maps to pywa's Order dataclass.
type Order struct {
	CatalogID    string
	Text         string
	ProductItems []OrderItem
}

// ── Unsupported ───────────────────────────────────────────────────────────────

// Unsupported represents a message type not supported by the Cloud API.
// Maps to pywa's Unsupported dataclass.
type Unsupported struct {
	MessageType string
}

// ── Contact ───────────────────────────────────────────────────────────────────

// ContactPhone is a phone number entry inside a Contact.
type ContactPhone struct {
	Phone string
	WAID  string
	Type  string
}

// ContactEmail is an email entry inside a Contact.
type ContactEmail struct {
	Email string
	Type  string
}

// ContactURL is a URL entry inside a Contact.
type ContactURL struct {
	URL  string
	Type string
}

// ContactAddress is a physical address inside a Contact.
type ContactAddress struct {
	Street      string
	City        string
	State       string
	Zip         string
	Country     string
	CountryCode string
	Type        string
}

// ContactName holds name components for a Contact.
type ContactName struct {
	FormattedName string
	FirstName     string
	LastName      string
	MiddleName    string
	Suffix        string
	Prefix        string
}

// ContactOrg holds organisation information for a Contact.
type ContactOrg struct {
	Company    string
	Department string
	Title      string
}

// Contact represents a rich contact card.
// Maps to pywa's Contact dataclass.
type Contact struct {
	Name      ContactName
	Phones    []ContactPhone
	Emails    []ContactEmail
	URLs      []ContactURL
	Addresses []ContactAddress
	Org       ContactOrg
	Birthday  string
}

// toDict serialises the Contact to a map suitable for the API JSON payload.
// Maps to pywa's Contact.to_dict().
//
// Returns:
//   - map[string]any ready for JSON marshalling.
func (c Contact) toDict() map[string]any {
	m := map[string]any{
		"name": map[string]any{
			"formatted_name": c.Name.FormattedName,
			"first_name":     c.Name.FirstName,
			"last_name":      c.Name.LastName,
			"middle_name":    c.Name.MiddleName,
			"suffix":         c.Name.Suffix,
			"prefix":         c.Name.Prefix,
		},
	}
	if len(c.Phones) > 0 {
		ps := make([]map[string]any, len(c.Phones))
		for i, p := range c.Phones {
			ps[i] = map[string]any{"phone": p.Phone, "wa_id": p.WAID, "type": p.Type}
		}
		m["phones"] = ps
	}
	if len(c.Emails) > 0 {
		es := make([]map[string]any, len(c.Emails))
		for i, e := range c.Emails {
			es[i] = map[string]any{"email": e.Email, "type": e.Type}
		}
		m["emails"] = es
	}
	if len(c.URLs) > 0 {
		us := make([]map[string]any, len(c.URLs))
		for i, u := range c.URLs {
			us[i] = map[string]any{"url": u.URL, "type": u.Type}
		}
		m["urls"] = us
	}
	if len(c.Addresses) > 0 {
		as := make([]map[string]any, len(c.Addresses))
		for i, a := range c.Addresses {
			as[i] = map[string]any{
				"street": a.Street, "city": a.City, "state": a.State,
				"zip": a.Zip, "country": a.Country, "country_code": a.CountryCode, "type": a.Type,
			}
		}
		m["addresses"] = as
	}
	if c.Org.Company != "" {
		m["org"] = map[string]any{
			"company": c.Org.Company, "department": c.Org.Department, "title": c.Org.Title,
		}
	}
	if c.Birthday != "" {
		m["birthday"] = c.Birthday
	}
	return m
}

// ── Media types ───────────────────────────────────────────────────────────────

// MediaBase holds common fields for all media types.
type MediaBase struct {
	ID       string // WhatsApp media ID
	SHA256   string
	MimeType string

	client *WhatsApp // for download shortcuts
}

// Image represents an image message attachment.
type Image struct {
	MediaBase
	Caption string
}

// Video represents a video message attachment.
type Video struct {
	MediaBase
	Caption string
}

// Audio represents an audio or voice message attachment.
type Audio struct {
	MediaBase
	Voice bool // true if this is a voice note (OGG/OPUS)
}

// Document represents a document attachment.
type Document struct {
	MediaBase
	Caption  string
	Filename string
}

// Sticker represents a sticker attachment.
type Sticker struct {
	MediaBase
	Animated bool
}

// ── MediaURL ──────────────────────────────────────────────────────────────────

// MediaURL holds a temporary (5-minute) download URL returned by the API.
// Maps to pywa's MediaURL dataclass.
type MediaURL struct {
	ID       string
	URL      string
	MimeType string
	SHA256   string
	FileSize int64

	client *WhatsApp
}

// ── Button types ──────────────────────────────────────────────────────────────

// Button is a quick-reply button (up to 3, max 20 chars label).
// Maps to pywa's Button dataclass.
type Button struct {
	// ID is the callback ID sent back when the user taps (max 256 chars).
	ID    string
	Title string
}

// URLButton opens a URL in a browser.
// Maps to pywa's URLButton dataclass.
type URLButton struct {
	Title string
	URL   string
}

// VoiceCallButton initiates a phone call when tapped.
// Maps to pywa's VoiceCallButton dataclass.
type VoiceCallButton struct {
	Title       string
	PhoneNumber string
}

// SectionRow is a row inside a list section.
type SectionRow struct {
	ID          string
	Title       string
	Description string
}

// Section represents one section in a list reply.
type Section struct {
	Title string
	Rows  []SectionRow
}

// SectionList is a list of sections displayed in a scrollable menu.
// Maps to pywa's SectionList dataclass.
type SectionList struct {
	ButtonText string
	Sections   []Section
}

// toDict serialises to the API action payload.
func (s SectionList) toDict() map[string]any {
	sections := make([]map[string]any, len(s.Sections))
	for i, sec := range s.Sections {
		rows := make([]map[string]any, len(sec.Rows))
		for j, r := range sec.Rows {
			rows[j] = map[string]any{"id": r.ID, "title": r.Title, "description": r.Description}
		}
		sections[i] = map[string]any{"title": sec.Title, "rows": rows}
	}
	return map[string]any{
		"button":   s.ButtonText,
		"sections": sections,
	}
}

// ProductsSection is a section of products in a product list message.
type ProductsSection struct {
	Title string
	SKUs  []string
}

// toDict converts to the API format.
func (p ProductsSection) toDict() map[string]any {
	items := make([]map[string]any, len(p.SKUs))
	for i, sku := range p.SKUs {
		items[i] = map[string]any{"product_retailer_id": sku}
	}
	return map[string]any{"title": p.Title, "product_items": items}
}

// FlowButton opens a WhatsApp Flow.
// Maps to pywa's FlowButton dataclass.
type FlowButton struct {
	FlowID       string
	FlowToken    string
	NavigateTo   string // screen name
	FlowActionID string
	FlowData     map[string]any
	Text         string // button label
}

// ── Callback types ────────────────────────────────────────────────────────────

// CallbackButton is received when a user taps a quick-reply button.
// Maps to pywa's CallbackButton update type.
type CallbackButton struct {
	BaseUserUpdate
	Title string
	Data  string // raw callback data
}

// CallbackSelection is received when a user selects from a list.
// Maps to pywa's CallbackSelection update type.
type CallbackSelection struct {
	BaseUserUpdate
	Title       string
	Data        string
	Description string
}

// ── Status ────────────────────────────────────────────────────────────────────

// MessageStatusType enumerates delivery statuses.
type MessageStatusType string

const (
	MessageStatusSent      MessageStatusType = "sent"
	MessageStatusDelivered MessageStatusType = "delivered"
	MessageStatusRead      MessageStatusType = "read"
	MessageStatusFailed    MessageStatusType = "failed"
	MessageStatusDeleted   MessageStatusType = "deleted"
	MessageStatusWarning   MessageStatusType = "warning"
)

// MessageStatus is fired when the delivery status of a sent message changes.
// Maps to pywa's MessageStatus update type.
type MessageStatus struct {
	ID        string
	Metadata  Metadata
	Status    MessageStatusType
	Timestamp time.Time
	From      User
	TrackerID string // biz_opaque_callback_data
	Error     *WhatsAppError
}

// ── Business account types ────────────────────────────────────────────────────

// BusinessProfile holds the public profile of a WhatsApp Business number.
// Maps to pywa's BusinessProfile dataclass.
type BusinessProfile struct {
	About            string
	Address          string
	Description      string
	Email            string
	Websites         []string
	VerticalName     string
	ProfilePictureID string
}

// BusinessPhoneNumber holds details about a business phone number.
// Maps to pywa's BusinessPhoneNumber dataclass.
type BusinessPhoneNumber struct {
	ID                     string
	DisplayPhoneNumber     string
	VerifiedName           string
	QualityRating          string
	CodeVerificationStatus string
	NameStatus             string
	IsOfficialBizAcct      bool
	AccountMode            string
}

// CommerceSettings holds catalog/cart settings.
// Maps to pywa's CommerceSettings dataclass.
type CommerceSettings struct {
	IsCatalogVisible bool
	IsCartEnabled    bool
}

// QRCode represents a WhatsApp QR code.
// Maps to pywa's QRCode dataclass.
type QRCode struct {
	Code             string
	PrefilledMessage string
	DeepLinkURL      string
	QRImageURL       string

	client  *WhatsApp
	phoneID string
}

// Delete deletes the QR code.
// Shortcut for WhatsApp.DeleteQRCode(code).
//
// Returns:
//   - error if deletion fails.
func (q *QRCode) Delete() error {
	if q.client == nil {
		return fmt.Errorf("qr code is not associated with a client")
	}
	return q.client.DeleteQRCode(q.Code)
}

// ── Flow types ────────────────────────────────────────────────────────────────

// FlowStatus is the current publication state of a flow.
type FlowStatus string

const (
	FlowStatusDraft      FlowStatus = "DRAFT"
	FlowStatusPublished  FlowStatus = "PUBLISHED"
	FlowStatusDeprecated FlowStatus = "DEPRECATED"
	FlowStatusBlocked    FlowStatus = "BLOCKED"
	FlowStatusThrottled  FlowStatus = "THROTTLED"
)

// FlowCategory classifies the purpose of a Flow.
type FlowCategory string

const (
	FlowCategorySignUp             FlowCategory = "SIGN_UP"
	FlowCategorySignIn             FlowCategory = "SIGN_IN"
	FlowCategoryAppointmentBooking FlowCategory = "APPOINTMENT_BOOKING"
	FlowCategoryLeadGeneration     FlowCategory = "LEAD_GENERATION"
	FlowCategoryContactUs          FlowCategory = "CONTACT_US"
	FlowCategoryCustomerSupport    FlowCategory = "CUSTOMER_SUPPORT"
	FlowCategorySurvey             FlowCategory = "SURVEY"
	FlowCategoryOther              FlowCategory = "OTHER"
)

// CreatedFlow is the response after creating a new flow.
type CreatedFlow struct {
	ID string
}

// FlowDetails holds full metadata for a flow.
type FlowDetails struct {
	ID               string
	Name             string
	Status           FlowStatus
	Categories       []FlowCategory
	ValidationErrors []map[string]any
	EndpointURI      string
	PreviewURL       string
}

// FlowRequest is sent to the business endpoint when a flow needs a data exchange.
// Maps to pywa's FlowRequest dataclass.
type FlowRequest struct {
	FlowToken       string
	Action          string
	Screen          string
	Data            map[string]any
	Version         string
	DecryptedAESKey []byte
	InitialVector   []byte
	PhoneNumberID   string
}

// FlowResponse is what the business server must return to a FlowRequest.
// Maps to pywa's FlowResponse dataclass.
type FlowResponse struct {
	Screen string
	Data   map[string]any
	Close  bool
}

// FlowCompletion is sent to the FlowCompletionHandler when a user completes a flow.
type FlowCompletion struct {
	BaseUserUpdate
	FlowToken string
	Response  map[string]any
}

// ── Template types ────────────────────────────────────────────────────────────

// TemplateLanguage is a BCP-47 language code supported by WhatsApp.
type TemplateLanguage string

// TemplateCategory is the category of a template, affecting review rules.
type TemplateCategory string

const (
	TemplateCategoryMarketing      TemplateCategory = "MARKETING"
	TemplateCategoryUtility        TemplateCategory = "UTILITY"
	TemplateCategoryAuthentication TemplateCategory = "AUTHENTICATION"
)

// TemplateStatus reflects the current review/approval status of a template.
type TemplateStatus string

const (
	TemplateStatusApproved        TemplateStatus = "APPROVED"
	TemplateStatusPaused          TemplateStatus = "PAUSED"
	TemplateStatusDisabled        TemplateStatus = "DISABLED"
	TemplateStatusPending         TemplateStatus = "PENDING"
	TemplateStatusRejected        TemplateStatus = "REJECTED"
	TemplateStatusPendingDeletion TemplateStatus = "PENDING_DELETION"
	TemplateStatusFlagged         TemplateStatus = "FLAGGED"
	TemplateStatusAppeal          TemplateStatus = "APPEAL_REQUESTED"
	TemplateStatusArchived        TemplateStatus = "ARCHIVED"
)

// CreatedTemplate is the API response after creating a template.
type CreatedTemplate struct {
	ID       string
	Status   TemplateStatus
	Category TemplateCategory
}

// TemplateParam is a single component parameter used when sending a template.
// Callers build these with the helpers in template_params.go.
type TemplateParam struct {
	Type string
	Raw  map[string]any
}

// ── Sent-message return types ─────────────────────────────────────────────────

// SentMessage is returned by all send* methods.
// Maps to pywa's SentMessage dataclass.
type SentMessage struct {
	ID          string
	FromPhoneID string
	To          string
	Timestamp   time.Time
}

// SentMediaMessage is returned by send-media methods.
// Extends SentMessage with the media ID for the uploaded media.
type SentMediaMessage struct {
	SentMessage
	MediaID string
}

// SentReaction is returned by SendReaction / RemoveReaction.
type SentReaction struct {
	SentMessage
	ReactedToMessageID string
}

// SentLocationRequest is returned by RequestLocation.
type SentLocationRequest struct {
	SentMessage
}

// SentTemplate is returned by SendTemplate.
type SentTemplate struct {
	SentMessage
}

// ── System update types ───────────────────────────────────────────────────────

// PhoneNumberChange fires when a user changes their phone number.
// Maps to pywa's PhoneNumberChange update.
type PhoneNumberChange struct {
	Metadata  Metadata
	Timestamp time.Time
	OldWAID   string
	NewWAID   string
}

// IdentityChange fires when a user's identity changes (device re-register).
// Maps to pywa's IdentityChange update.
type IdentityChange struct {
	Metadata         Metadata
	Timestamp        time.Time
	From             User
	CreatedTimestamp time.Time
	Hash             string
}

// ChatOpened fires the first time a user opens a chat with the business.
// Maps to pywa's ChatOpened update.
type ChatOpened struct {
	Metadata  Metadata
	Timestamp time.Time
	From      User
}

// ── Template update events ────────────────────────────────────────────────────

// TemplateStatusUpdate fires when a template's approval status changes.
type TemplateStatusUpdate struct {
	TemplateID   string
	TemplateName string
	Status       TemplateStatus
	Reason       string
}

// TemplateCategoryUpdate fires when a template's category is changed by Meta.
type TemplateCategoryUpdate struct {
	TemplateID       string
	TemplateName     string
	PreviousCategory TemplateCategory
	NewCategory      TemplateCategory
}

// TemplateQualityUpdate fires when a template's quality score changes.
type TemplateQualityUpdate struct {
	TemplateID   string
	TemplateName string
	QualityScore string
}

// TemplateComponentsUpdate fires when a template's components are updated.
type TemplateComponentsUpdate struct {
	TemplateID string
}

// UserMarketingPreferences fires when a user's marketing opt-in status changes.
type UserMarketingPreferences struct {
	Metadata  Metadata
	Timestamp time.Time
	From      User
	OptIn     bool
}

// ── Calling types ─────────────────────────────────────────────────────────────

// CallStatus holds delivery status for a WhatsApp call update.
type CallStatus struct {
	CallID    string
	Status    string
	From      User
	Timestamp time.Time
}

// CallConnect fires when an inbound call is connected.
type CallConnect struct {
	CallID    string
	From      User
	Timestamp time.Time
}

// CallTerminate fires when a call is terminated.
type CallTerminate struct {
	CallID    string
	From      User
	Duration  int
	Timestamp time.Time
}

// CallPermissionRequestButton is a button that prompts the user to grant call permission.
type CallPermissionRequestButton struct {
	Title string
}

// CallPermissionUpdate fires when a user responds to a call permission request.
type CallPermissionUpdate struct {
	BaseUserUpdate
	Response string
}

// SessionDescription holds the SDP for a WebRTC call.
type SessionDescription struct {
	Type string
	SDP  string
}

// toDict serialises the SDP for the API payload.
func (s SessionDescription) toDict() map[string]any {
	return map[string]any{"type": s.Type, "sdp": s.SDP}
}

// InitiatedCall is returned when a call is successfully initiated.
type InitiatedCall struct {
	SentMessage
	CallID string
}

// ── Pagination ────────────────────────────────────────────────────────────────

// Pagination controls paging for list API calls.
// Maps to pywa's Pagination dataclass.
type Pagination struct {
	Limit  int
	After  string
	Before string
}

// toDict converts to query params for the API.
func (p *Pagination) toDict() map[string]string {
	m := map[string]string{}
	if p == nil {
		return m
	}
	if p.Limit > 0 {
		m["limit"] = fmt.Sprintf("%d", p.Limit)
	}
	if p.After != "" {
		m["after"] = p.After
	}
	if p.Before != "" {
		m["before"] = p.Before
	}
	return m
}

// ── Result / cursor-paginated list ────────────────────────────────────────────

// Result is a cursor-paginated result set for any item type T.
// Mirrors pywa's Result[T] generic.
//
// Use NextPage to retrieve the next page, or iterate Items directly.
type Result[T any] struct {
	Items      []T
	TotalCount int
	NextCursor string
	PrevCursor string

	// internal state for automatic paging
	wa      *WhatsApp
	fetchFn func(after string) (*Result[T], error)
}

// HasNextPage returns true if there is another page of results.
func (r *Result[T]) HasNextPage() bool {
	return r.NextCursor != ""
}

// NextPage fetches the next page of results.
//
// Returns:
//   - *Result[T] with the next page items.
//   - error if the request fails or there is no next page.
func (r *Result[T]) NextPage() (*Result[T], error) {
	if !r.HasNextPage() {
		return nil, fmt.Errorf("no next page")
	}
	if r.fetchFn == nil {
		return nil, fmt.Errorf("pagination not supported for this result")
	}
	return r.fetchFn(r.NextCursor)
}

// ── WhatsApp Business Account ─────────────────────────────────────────────────

// WhatsAppBusinessAccount holds WABA-level information.
type WhatsAppBusinessAccount struct {
	ID                       string
	Name                     string
	Currency                 string
	MessageTemplateNamespace string
}

// ── Success result ────────────────────────────────────────────────────────────

// SuccessResult is returned for API calls that only indicate success/failure.
// Maps to pywa's SuccessResult.
type SuccessResult struct {
	Success bool
}

// ── User block/unblock results ────────────────────────────────────────────────

// BlockedUser is a single entry in UsersBlockedResult.
type BlockedUser struct {
	WAID  string
	Input string
}

// UsersBlockedResult is returned by BlockUsers.
type UsersBlockedResult struct {
	AddedUsers  []BlockedUser
	FailedUsers []BlockedUser
}

// UnblockedUser is a single entry in UsersUnblockedResult.
type UnblockedUser struct {
	WAID string
}

// UsersUnblockedResult is returned by UnblockUsers.
type UsersUnblockedResult struct {
	RemovedUsers []UnblockedUser
}

// ── Command ───────────────────────────────────────────────────────────────────

// Command is a slash-command shown in WhatsApp chat when a user types '/'.
// Maps to pywa's Command dataclass.
type Command struct {
	Command     string
	Description string
}

// toDict serialises for the API payload.
func (c Command) toDict() map[string]any {
	return map[string]any{"command_name": c.Command, "command_description": c.Description}
}

// ── Raw update ────────────────────────────────────────────────────────────────

// RawUpdate is the decoded top-level webhook payload, before classification.
// Handlers registered with OnRawUpdate receive this.
type RawUpdate map[string]any

// ── StorageConfiguration ──────────────────────────────────────────────────────

// StorageConfiguration controls the no-storage configuration of a phone number.
type StorageConfiguration struct {
	StorageType string // "EPHEMERAL" or "PERSISTENT"
}

// ── Calling / business settings ───────────────────────────────────────────────

// CallingSettings controls calling on a business phone number.
type CallingSettings struct {
	Status string // "ENABLED" | "DISABLED"
}
