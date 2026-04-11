package gowa

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────────
// Remaining client methods — everything not yet in client.go or client_extended.go
// ────────────────────────────────────────────────────────────────────────────────

// ── StreamMedia ───────────────────────────────────────────────────────────────

// StreamMedia returns an io.ReadCloser that streams media bytes from a WhatsApp
// media URL.  The caller is responsible for closing the reader.
// Mirrors pywa's WhatsApp.stream_media.
//
// Parameters:
//   - mediaURL: temporary URL obtained from GetMediaURL.
//
// Returns:
//   - io.ReadCloser: the streaming body; must be closed by the caller.
//   - error if the request fails.
//
// Example:
//
//	rc, err := wa.StreamMedia(mediaURL.URL)
//	if err != nil { ... }
//	defer rc.Close()
//	http.Post("https://my-server.com/upload", "application/octet-stream", rc)
func (wa *WhatsApp) StreamMedia(mediaURL string) (io.ReadCloser, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	req, err := newAuthRequest(wa.api.token, mediaURL)
	if err != nil {
		return nil, err
	}
	resp, err := wa.api.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gowa: StreamMedia: %w", err)
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("gowa: StreamMedia: HTTP %d for %s", resp.StatusCode, mediaURL)
	}
	return resp.Body, nil
}

// ── Business phone number settings ───────────────────────────────────────────

// BusinessPhoneNumberSettings holds advanced settings for a business phone number.
// Maps to pywa's BusinessPhoneNumberSettings.
type BusinessPhoneNumberSettings struct {
	// Calling holds the calling-feature configuration.
	Calling *CallingSettings
	// StorageConfiguration controls the no-storage option.
	StorageConfiguration *StorageConfiguration
	// WebhookConfiguration holds the current phone-level webhook override (if any).
	WebhookConfiguration map[string]any
}

// GetBusinessPhoneNumberSettings fetches advanced settings for a business phone
// number, such as calling configuration and storage mode.
// Mirrors pywa's WhatsApp.get_business_phone_number_settings.
//
// Parameters:
//   - phoneID: optional phone number ID override.
//
// Returns:
//   - *BusinessPhoneNumberSettings.
//   - error.
func (wa *WhatsApp) GetBusinessPhoneNumberSettings(phoneID string) (*BusinessPhoneNumberSettings, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}
	pid, err := wa.resolveSender(phoneID)
	if err != nil {
		return nil, err
	}
	fields := "calling,storage_configuration,webhook_configuration"
	res, err := wa.api.getBusinessPhoneNumber(pid, fields)
	if err != nil {
		return nil, err
	}

	s := &BusinessPhoneNumberSettings{}
	if calling := toMap(res["calling"]); calling != nil {
		s.Calling = &CallingSettings{
			Status: toString(calling["status"]),
		}
	}
	if sc := toMap(res["storage_configuration"]); sc != nil {
		s.StorageConfiguration = &StorageConfiguration{
			StorageType: toString(sc["storage_type"]),
		}
	}
	if wc := toMap(res["webhook_configuration"]); wc != nil {
		s.WebhookConfiguration = wc
	}
	return s, nil
}

// UpdateBusinessPhoneNumberSettingsOptions holds the fields that can be updated.
type UpdateBusinessPhoneNumberSettingsOptions struct {
	// Calling updates the calling-feature status ("ENABLED" | "DISABLED").
	Calling *CallingSettings
	// StorageConfiguration updates the storage mode.
	StorageConfiguration *StorageConfiguration
	// PhoneID overrides the client's phone number ID.
	PhoneID string
}

// UpdateBusinessPhoneNumberSettings updates one or more settings on the business
// phone number.
// Mirrors pywa's WhatsApp.update_business_phone_number_settings.
//
// Parameters:
//   - opts: UpdateBusinessPhoneNumberSettingsOptions with the fields to change.
//
// Returns:
//   - error.
func (wa *WhatsApp) UpdateBusinessPhoneNumberSettings(opts UpdateBusinessPhoneNumberSettingsOptions) error {
	if err := wa.requireAPI(); err != nil {
		return err
	}
	pid, err := wa.resolveSender(opts.PhoneID)
	if err != nil {
		return err
	}
	body := map[string]any{}
	if opts.Calling != nil {
		body["calling"] = map[string]any{"status": opts.Calling.Status}
	}
	if opts.StorageConfiguration != nil {
		body["storage_configuration"] = map[string]any{
			"storage_type": opts.StorageConfiguration.StorageType,
		}
	}
	if len(body) == 0 {
		return fmt.Errorf("gowa: UpdateBusinessPhoneNumberSettings: at least one option must be provided")
	}
	_, apiErr := wa.api.post("/"+pid, body)
	return apiErr
}

// ── GetFlowMetrics ────────────────────────────────────────────────────────────

// FlowMetricName is the metric to retrieve for a flow.
type FlowMetricName string

const (
	FlowMetricSent         FlowMetricName = "SENT"
	FlowMetricOpened       FlowMetricName = "OPENED"
	FlowMetricCompleted    FlowMetricName = "COMPLETED"
	FlowMetricUniqueOpened FlowMetricName = "UNIQUE_OPENED"
)

// FlowMetricGranularity is the time bucket size for flow metrics.
type FlowMetricGranularity string

const (
	FlowMetricGranularityDay  FlowMetricGranularity = "DAY"
	FlowMetricGranularityHour FlowMetricGranularity = "HOUR"
)

// FlowMetricOptions holds optional date-range parameters for GetFlowMetrics.
type FlowMetricOptions struct {
	// Since is the start of the time range (optional; defaults to oldest allowed).
	Since *time.Time
	// Until is the end of the time range (optional; defaults to today).
	Until *time.Time
}

// GetFlowMetrics retrieves engagement metrics for a published flow.
// Mirrors pywa's WhatsApp.get_flow_metrics.
//
// Parameters:
//   - flowID:      the flow ID.
//   - metricName:  which metric to retrieve (e.g. FlowMetricSent).
//   - granularity: time bucketing (FlowMetricGranularityDay | FlowMetricGranularityHour).
//   - opts:        optional date-range filter.
//
// Returns:
//   - map[string]any: raw metric data from the API.
//   - error.
func (wa *WhatsApp) GetFlowMetrics(flowID string, metricName FlowMetricName, granularity FlowMetricGranularity, opts ...FlowMetricOptions) (map[string]any, error) {
	if err := wa.requireAPI(); err != nil {
		return nil, err
	}

	// Build the nested field selector that pywa uses:
	// "metric.name(SENT).granularity(DAY)[.since(TS)][.until(TS)]"
	field := fmt.Sprintf("metric.name(%s).granularity(%s)", metricName, granularity)
	if len(opts) > 0 {
		o := opts[0]
		if o.Since != nil {
			field += fmt.Sprintf(".since(%d)", o.Since.Unix())
		}
		if o.Until != nil {
			field += fmt.Sprintf(".until(%d)", o.Until.Unix())
		}
	}

	res, err := wa.api.getFlow(flowID, field)
	if err != nil {
		return nil, err
	}
	metric, _ := res["metric"].(map[string]any)
	if metric == nil {
		return map[string]any{}, nil
	}
	return metric, nil
}

// ── Handler management aliases ────────────────────────────────────────────────
// pywa exposes add_handlers / remove_handlers / remove_callbacks as generic
// wrappers over all handler types.  In Go we type-check at registration time,
// so we provide typed helpers per update kind plus generic HandlerSpec wrappers.

// HandlerSpec is a sealed interface for all handler specifications.
// Build one with MessageHandlerSpec(), CallbackButtonHandlerSpec(), etc.
type HandlerSpec interface {
	register(wa *WhatsApp) error
	unregister(wa *WhatsApp) error
}

// ── Per-type HandlerSpec implementations ─────────────────────────────────────

type messageHandlerSpec struct {
	fn       func(*WhatsApp, *Message)
	filters  []Filter[*Message]
	priority int
}

// MessageHandlerSpec creates a HandlerSpec for message handlers.
// Use with AddHandlers / RemoveHandlers.
//
// Parameters:
//   - fn:       the handler callback.
//   - priority: handlers with higher priority run first.
//   - filters:  optional filter predicates.
//
// Returns:
//   - HandlerSpec ready for AddHandlers.
func MessageHandlerSpec(fn func(*WhatsApp, *Message), priority int, filters ...Filter[*Message]) HandlerSpec {
	return &messageHandlerSpec{fn: fn, filters: filters, priority: priority}
}

func (s *messageHandlerSpec) register(wa *WhatsApp) error {
	return wa.addMessageHandler(s.fn, s.priority, s.filters...)
}
func (s *messageHandlerSpec) unregister(wa *WhatsApp) error {
	return wa.hdlrs.message.remove(s.fn)
}

type callbackButtonHandlerSpec struct {
	fn      func(*WhatsApp, *CallbackButton)
	filters []Filter[*CallbackButton]
}

// CallbackButtonHandlerSpec creates a HandlerSpec for callback-button handlers.
//
// Parameters:
//   - fn:      the handler callback.
//   - filters: optional filter predicates.
//
// Returns:
//   - HandlerSpec ready for AddHandlers.
func CallbackButtonHandlerSpec(fn func(*WhatsApp, *CallbackButton), filters ...Filter[*CallbackButton]) HandlerSpec {
	return &callbackButtonHandlerSpec{fn: fn, filters: filters}
}

func (s *callbackButtonHandlerSpec) register(wa *WhatsApp) error {
	return wa.OnCallbackButton(s.fn, s.filters...)
}
func (s *callbackButtonHandlerSpec) unregister(wa *WhatsApp) error {
	return wa.hdlrs.callbackButton.remove(s.fn)
}

type callbackSelectionHandlerSpec struct {
	fn      func(*WhatsApp, *CallbackSelection)
	filters []Filter[*CallbackSelection]
}

// CallbackSelectionHandlerSpec creates a HandlerSpec for list-selection handlers.
func CallbackSelectionHandlerSpec(fn func(*WhatsApp, *CallbackSelection), filters ...Filter[*CallbackSelection]) HandlerSpec {
	return &callbackSelectionHandlerSpec{fn: fn, filters: filters}
}
func (s *callbackSelectionHandlerSpec) register(wa *WhatsApp) error {
	return wa.OnCallbackSelection(s.fn, s.filters...)
}
func (s *callbackSelectionHandlerSpec) unregister(wa *WhatsApp) error {
	return wa.hdlrs.callbackSelect.remove(s.fn)
}

type messageStatusHandlerSpec struct {
	fn      func(*WhatsApp, *MessageStatus)
	filters []Filter[*MessageStatus]
}

// MessageStatusHandlerSpec creates a HandlerSpec for message-status handlers.
func MessageStatusHandlerSpec(fn func(*WhatsApp, *MessageStatus), filters ...Filter[*MessageStatus]) HandlerSpec {
	return &messageStatusHandlerSpec{fn: fn, filters: filters}
}
func (s *messageStatusHandlerSpec) register(wa *WhatsApp) error {
	return wa.OnMessageStatus(s.fn, s.filters...)
}
func (s *messageStatusHandlerSpec) unregister(wa *WhatsApp) error {
	return wa.hdlrs.messageStatus.remove(s.fn)
}

// ── AddHandlers / RemoveHandlers / RemoveCallbacks / LoadHandlersModules ─────

// AddHandlers registers one or more HandlerSpecs with the client.
// This is the programmatic equivalent of pywa's wa.add_handlers().
//
// Parameters:
//   - specs: one or more HandlerSpec values built with *HandlerSpec helpers.
//
// Returns:
//   - error if the client is not configured for webhooks, or any spec fails.
//
// Example:
//
//	wa.AddHandlers(
//	    gowa.MessageHandlerSpec(onText, 0, gowa.FilterText),
//	    gowa.CallbackButtonHandlerSpec(onButton),
//	)
func (wa *WhatsApp) AddHandlers(specs ...HandlerSpec) error {
	for _, s := range specs {
		if err := s.register(wa); err != nil {
			return err
		}
	}
	return nil
}

// RemoveHandlers removes one or more HandlerSpecs that were previously registered
// via AddHandlers.
// Mirrors pywa's wa.remove_handlers().
//
// Parameters:
//   - specs: the same HandlerSpec values passed to AddHandlers.
//   - silent: if true, missing handlers are silently ignored.
//
// Returns:
//   - error if a spec was not registered and silent is false.
func (wa *WhatsApp) RemoveHandlers(silent bool, specs ...HandlerSpec) error {
	for _, s := range specs {
		if err := s.unregister(wa); err != nil && !silent {
			return err
		}
	}
	return nil
}

// RemoveCallbacks removes all message handlers whose callback matches any of the
// provided functions.
// Mirrors pywa's wa.remove_callbacks().
//
// Parameters:
//   - callbacks: function pointers previously passed to OnMessage / AddMessageHandler.
func (wa *WhatsApp) RemoveCallbacks(callbacks ...func(*WhatsApp, *Message)) {
	for _, cb := range callbacks {
		_ = wa.hdlrs.message.remove(cb)
	}
}

// HandlerModule is any type that carries pre-registered handler specs.
// Implement this interface on a struct to use LoadHandlersModules.
//
// Example:
//
//	type MyHandlers struct{}
//	func (MyHandlers) Handlers() []gowa.HandlerSpec {
//	    return []gowa.HandlerSpec{
//	        gowa.MessageHandlerSpec(onText, 0, gowa.FilterText),
//	    }
//	}
type HandlerModule interface {
	Handlers() []HandlerSpec
}

// LoadHandlersModules registers all handlers declared in each provided module.
// This is the Go equivalent of pywa's wa.load_handlers_modules(*modules).
//
// Parameters:
//   - modules: one or more HandlerModule implementations.
//
// Returns:
//   - error if the client is not configured for webhooks, or any registration fails.
//
// Example:
//
//	type BotHandlers struct{}
//	func (BotHandlers) Handlers() []gowa.HandlerSpec {
//	    return []gowa.HandlerSpec{
//	        gowa.MessageHandlerSpec(onText, 0, gowa.FilterText),
//	        gowa.CallbackButtonHandlerSpec(onButton),
//	    }
//	}
//
//	wa.LoadHandlersModules(BotHandlers{})
func (wa *WhatsApp) LoadHandlersModules(modules ...HandlerModule) error {
	for _, mod := range modules {
		if err := wa.AddHandlers(mod.Handlers()...); err != nil {
			return err
		}
	}
	return nil
}

// ── newAuthRequest helper ─────────────────────────────────────────────────────

// newAuthRequest builds an authenticated GET request for a media URL.
// Used by StreamMedia.
//
// Parameters:
//   - token:    bearer access token.
//   - mediaURL: the full URL to request.
//
// Returns:
//   - *http.Request with the Authorization header set.
//   - error if the URL is invalid.
func newAuthRequest(token, mediaURL string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}
