package gowa

import (
	"fmt"
	"sort"
	"sync"
)

// ────────────────────────────────────────────────────────────────────────────────
// Handler system
// Python's decorator pattern:
//
//   @wa.on_message(filters.text)
//   def on_text(wa, msg): ...
//
// becomes in Go:
//
//   wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
//       // ...
//   }, gowa.FilterText)
//
// Handlers can also be built explicitly and added via AddHandlers:
//
//   h := gowa.NewMessageHandler(callback, priority, filters...)
//   wa.AddHandlers(h)
//
// ────────────────────────────────────────────────────────────────────────────────

// handlerEntry is an internal container that holds one registered handler.
// It carries a priority so handlers run in order (highest priority first).
type handlerEntry[T any] struct {
	callback func(*WhatsApp, T)
	filters  []Filter[T]
	priority int
}

// matches returns true when every filter in the entry passes for the given update.
//
// Parameters:
//   - wa:     the active WhatsApp client.
//   - update: the update value to test.
//
// Returns:
//   - true if all filters pass (or there are no filters).
func (h *handlerEntry[T]) matches(wa *WhatsApp, update T) bool {
	for _, f := range h.filters {
		if !f(wa, update) {
			return false
		}
	}
	return true
}

// handlerList is a sorted list of handler entries for a single update type.
type handlerList[T any] struct {
	mu       sync.RWMutex
	handlers []*handlerEntry[T]
}

// add inserts a new handler, maintaining descending priority order.
//
// Parameters:
//   - h: the handlerEntry to insert.
func (l *handlerList[T]) add(h *handlerEntry[T]) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.handlers = append(l.handlers, h)
	sort.Slice(l.handlers, func(i, j int) bool {
		return l.handlers[i].priority > l.handlers[j].priority
	})
}

// remove deletes the handler entry whose callback pointer matches cb.
// Returns an error when the callback is not found.
//
// Parameters:
//   - cb: function pointer of the handler to remove.
//
// Returns:
//   - error if no matching handler is found.
func (l *handlerList[T]) remove(cb func(*WhatsApp, T)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, h := range l.handlers {
		if fmt.Sprintf("%p", h.callback) == fmt.Sprintf("%p", cb) {
			l.handlers = append(l.handlers[:i], l.handlers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("handler not registered")
}

// dispatch calls every matching handler for update.
// If wa.continueHandling is false, dispatch stops after the first match.
//
// Parameters:
//   - wa:     the active client (passed to each callback).
//   - update: the update value to dispatch.
func (l *handlerList[T]) dispatch(wa *WhatsApp, update T) {
	l.mu.RLock()
	handlers := make([]*handlerEntry[T], len(l.handlers))
	copy(handlers, l.handlers)
	l.mu.RUnlock()

	for _, h := range handlers {
		if h.matches(wa, update) {
			h.callback(wa, update)
			if !wa.continueHandling {
				return
			}
		}
	}
}

// ── Per-type handler stores held inside WhatsApp ──────────────────────────────

// handlers is the top-level struct that owns all per-type lists.
type handlers struct {
	message        handlerList[*Message]
	callbackButton handlerList[*CallbackButton]
	callbackSelect handlerList[*CallbackSelection]
	messageStatus  handlerList[*MessageStatus]
	chatOpened     handlerList[*ChatOpened]
	flowCompletion handlerList[*FlowCompletion]
	flowRequest    handlerList[*FlowRequest]
	phoneNumChange handlerList[*PhoneNumberChange]
	identityChange handlerList[*IdentityChange]
	tmplStatus     handlerList[*TemplateStatusUpdate]
	tmplCategory   handlerList[*TemplateCategoryUpdate]
	tmplQuality    handlerList[*TemplateQualityUpdate]
	tmplComponents handlerList[*TemplateComponentsUpdate]
	userMktgPrefs  handlerList[*UserMarketingPreferences]
	callConnect    handlerList[*CallConnect]
	callTerminate  handlerList[*CallTerminate]
	callStatus     handlerList[*CallStatus]
	callPermission handlerList[*CallPermissionUpdate]
	raw            handlerList[RawUpdate]
}

// ── Public registration methods on WhatsApp ───────────────────────────────────
// These replace pywa's @wa.on_message / @wa.on_callback_button decorators.

// OnMessage registers a callback for incoming text/media/contact/location messages.
// The optional filters are ANDed together; the callback fires only when all pass.
// Priority defaults to 0; use AddHandlers with a custom handlerEntry for ordering.
//
// Parameters:
//   - callback: func(*WhatsApp, *Message) invoked when a matching update arrives.
//   - filters:  zero or more Filter[*Message]; all must pass (logical AND).
//
// Returns:
//   - error if the client is not set up to receive updates.
//
// Example:
//
//	wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
//	    fmt.Println(*msg.Text)
//	}, gowa.FilterText)
func (wa *WhatsApp) OnMessage(callback func(*WhatsApp, *Message), filters ...Filter[*Message]) error {
	return wa.addMessageHandler(callback, 0, filters...)
}

// AddMessageHandler registers a message handler with an explicit priority.
// Higher priority handlers run first.
//
// Parameters:
//   - callback: the handler function.
//   - priority: integer priority; larger = called first.
//   - filters:  optional filters (ANDed).
//
// Returns:
//   - error if the client is not configured to receive webhooks.
func (wa *WhatsApp) AddMessageHandler(callback func(*WhatsApp, *Message), priority int, filters ...Filter[*Message]) error {
	return wa.addMessageHandler(callback, priority, filters...)
}

func (wa *WhatsApp) addMessageHandler(callback func(*WhatsApp, *Message), priority int, filters ...Filter[*Message]) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.message.add(&handlerEntry[*Message]{
		callback: callback,
		filters:  filters,
		priority: priority,
	})
	return nil
}

// RemoveMessageHandler removes a previously registered message handler by its
// callback function pointer.
//
// Parameters:
//   - callback: the exact function pointer passed to OnMessage / AddMessageHandler.
//
// Returns:
//   - error if the callback is not registered.
func (wa *WhatsApp) RemoveMessageHandler(callback func(*WhatsApp, *Message)) error {
	return wa.hdlrs.message.remove(callback)
}

// OnCallbackButton registers a handler for quick-reply button taps.
//
// Parameters:
//   - callback: func(*WhatsApp, *CallbackButton) invoked on a button tap.
//   - filters:  optional Filter[*CallbackButton] predicates.
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnCallbackButton(callback func(*WhatsApp, *CallbackButton), filters ...Filter[*CallbackButton]) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.callbackButton.add(&handlerEntry[*CallbackButton]{
		callback: callback,
		filters:  filters,
	})
	return nil
}

// RemoveCallbackButtonHandler removes a callback-button handler by function pointer.
func (wa *WhatsApp) RemoveCallbackButtonHandler(cb func(*WhatsApp, *CallbackButton)) error {
	return wa.hdlrs.callbackButton.remove(cb)
}

// OnCallbackSelection registers a handler for list-selection replies.
//
// Parameters:
//   - callback: func(*WhatsApp, *CallbackSelection) invoked on a list selection.
//   - filters:  optional Filter[*CallbackSelection] predicates.
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnCallbackSelection(callback func(*WhatsApp, *CallbackSelection), filters ...Filter[*CallbackSelection]) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.callbackSelect.add(&handlerEntry[*CallbackSelection]{
		callback: callback,
		filters:  filters,
	})
	return nil
}

// OnMessageStatus registers a handler for delivery/read status changes.
//
// Parameters:
//   - callback: func(*WhatsApp, *MessageStatus) invoked on a status update.
//   - filters:  optional Filter[*MessageStatus] predicates.
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnMessageStatus(callback func(*WhatsApp, *MessageStatus), filters ...Filter[*MessageStatus]) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.messageStatus.add(&handlerEntry[*MessageStatus]{
		callback: callback,
		filters:  filters,
	})
	return nil
}

// OnChatOpened registers a handler for the first-time chat-opened event.
//
// Parameters:
//   - callback: func(*WhatsApp, *ChatOpened).
//   - filters:  optional filters.
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnChatOpened(callback func(*WhatsApp, *ChatOpened), filters ...Filter[*ChatOpened]) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.chatOpened.add(&handlerEntry[*ChatOpened]{callback: callback, filters: filters})
	return nil
}

// OnFlowCompletion registers a handler called when a user completes a WhatsApp Flow.
//
// Parameters:
//   - callback: func(*WhatsApp, *FlowCompletion).
//   - filters:  optional filters.
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnFlowCompletion(callback func(*WhatsApp, *FlowCompletion), filters ...Filter[*FlowCompletion]) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.flowCompletion.add(&handlerEntry[*FlowCompletion]{callback: callback, filters: filters})
	return nil
}

// OnPhoneNumberChange registers a handler for user phone-number change events.
//
// Parameters:
//   - callback: func(*WhatsApp, *PhoneNumberChange).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnPhoneNumberChange(callback func(*WhatsApp, *PhoneNumberChange)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.phoneNumChange.add(&handlerEntry[*PhoneNumberChange]{callback: callback})
	return nil
}

// OnIdentityChange registers a handler for user device-identity change events.
//
// Parameters:
//   - callback: func(*WhatsApp, *IdentityChange).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnIdentityChange(callback func(*WhatsApp, *IdentityChange)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.identityChange.add(&handlerEntry[*IdentityChange]{callback: callback})
	return nil
}

// OnTemplateStatusUpdate registers a handler for template approval status changes.
//
// Parameters:
//   - callback: func(*WhatsApp, *TemplateStatusUpdate).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnTemplateStatusUpdate(callback func(*WhatsApp, *TemplateStatusUpdate)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.tmplStatus.add(&handlerEntry[*TemplateStatusUpdate]{callback: callback})
	return nil
}

// OnTemplateCategoryUpdate registers a handler for template category changes.
//
// Parameters:
//   - callback: func(*WhatsApp, *TemplateCategoryUpdate).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnTemplateCategoryUpdate(callback func(*WhatsApp, *TemplateCategoryUpdate)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.tmplCategory.add(&handlerEntry[*TemplateCategoryUpdate]{callback: callback})
	return nil
}

// OnTemplateQualityUpdate registers a handler for template quality-score changes.
//
// Parameters:
//   - callback: func(*WhatsApp, *TemplateQualityUpdate).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnTemplateQualityUpdate(callback func(*WhatsApp, *TemplateQualityUpdate)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.tmplQuality.add(&handlerEntry[*TemplateQualityUpdate]{callback: callback})
	return nil
}

// OnUserMarketingPreferences registers a handler for marketing opt-in/out events.
//
// Parameters:
//   - callback: func(*WhatsApp, *UserMarketingPreferences).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnUserMarketingPreferences(callback func(*WhatsApp, *UserMarketingPreferences)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.userMktgPrefs.add(&handlerEntry[*UserMarketingPreferences]{callback: callback})
	return nil
}

// OnCallConnect registers a handler for inbound call connection events.
//
// Parameters:
//   - callback: func(*WhatsApp, *CallConnect).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnCallConnect(callback func(*WhatsApp, *CallConnect)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.callConnect.add(&handlerEntry[*CallConnect]{callback: callback})
	return nil
}

// OnCallTerminate registers a handler for call termination events.
//
// Parameters:
//   - callback: func(*WhatsApp, *CallTerminate).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnCallTerminate(callback func(*WhatsApp, *CallTerminate)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.callTerminate.add(&handlerEntry[*CallTerminate]{callback: callback})
	return nil
}

// OnCallStatus registers a handler for call status updates.
//
// Parameters:
//   - callback: func(*WhatsApp, *CallStatus).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnCallStatus(callback func(*WhatsApp, *CallStatus)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.callStatus.add(&handlerEntry[*CallStatus]{callback: callback})
	return nil
}

// OnCallPermissionUpdate registers a handler for call permission reply events.
//
// Parameters:
//   - callback: func(*WhatsApp, *CallPermissionUpdate).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnCallPermissionUpdate(callback func(*WhatsApp, *CallPermissionUpdate)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.callPermission.add(&handlerEntry[*CallPermissionUpdate]{callback: callback})
	return nil
}

// OnRawUpdate registers a handler that receives the raw decoded JSON payload
// of every incoming webhook update, regardless of type.
//
// Parameters:
//   - callback: func(*WhatsApp, RawUpdate).
//
// Returns:
//   - error if the client is not set up to receive updates.
func (wa *WhatsApp) OnRawUpdate(callback func(*WhatsApp, RawUpdate)) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.hdlrs.raw.add(&handlerEntry[RawUpdate]{callback: callback})
	return nil
}

// ── Flow request handler ──────────────────────────────────────────────────────

// FlowRequestHandlerFunc is the signature for a WhatsApp Flow request handler.
// The function must return a FlowResponse to continue the flow, or an error.
//
// Parameters:
//   - wa:  the active WhatsApp client.
//   - req: the decrypted FlowRequest from the user.
//
// Returns:
//   - *FlowResponse: the response payload to encrypt and return.
//   - error: non-nil causes gowa to return an HTTP 500 to WhatsApp.
type FlowRequestHandlerFunc func(wa *WhatsApp, req *FlowRequest) (*FlowResponse, error)

// RegisterFlowEndpoint registers a handler for WhatsApp Flow data-exchange requests
// arriving at the given URL path.
//
// Parameters:
//   - endpoint: the HTTP path to handle (e.g. "/flows/survey").
//   - handler:  the FlowRequestHandlerFunc to call.
//
// Returns:
//   - error if the client is not set up to serve a webhook.
func (wa *WhatsApp) RegisterFlowEndpoint(endpoint string, handler FlowRequestHandlerFunc) error {
	if err := wa.requireWebhook(); err != nil {
		return err
	}
	wa.mu.Lock()
	defer wa.mu.Unlock()
	if wa.flowEndpoints == nil {
		wa.flowEndpoints = map[string]FlowRequestHandlerFunc{}
	}
	wa.flowEndpoints[endpoint] = handler
	return nil
}
