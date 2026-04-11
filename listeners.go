package gowa

import (
	"fmt"
	"sync"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────────
// Listener system
// Mirrors pywa's listeners.py — a blocking "listen for the next matching update"
// primitive used to implement conversational flows inside handlers.
//
// Python usage:
//
//	update = wa.listen(to=UserUpdateListenerIdentifier(sender="123", recipient="456"),
//	                   filters=filters.text, timeout=30)
//
// Go equivalent:
//
//	update, err := wa.Listen(gowa.ListenOptions{
//	    SenderWAID:    "123",
//	    RecipientID:   "456",
//	    Filters:       gowa.FilterText,
//	    Timeout:       30 * time.Second,
//	})
// ────────────────────────────────────────────────────────────────────────────────

// ListenerTimeout is returned (as an error) when no matching update arrives
// before the deadline.
// Mirrors pywa's ListenerTimeout exception.
type ListenerTimeout struct {
	Duration time.Duration
}

// Error implements the error interface.
func (e *ListenerTimeout) Error() string {
	return fmt.Sprintf("gowa: listener timed out after %v", e.Duration)
}

// ListenerCanceled is returned when a canceler filter matched an incoming update,
// meaning the user explicitly cancelled the conversational step.
// Mirrors pywa's ListenerCanceled exception.
type ListenerCanceled struct {
	// Update is the update that triggered the cancellation (may be nil).
	Update any
}

// Error implements the error interface.
func (e *ListenerCanceled) Error() string {
	if e.Update != nil {
		return fmt.Sprintf("gowa: listener cancelled by update: %v", e.Update)
	}
	return "gowa: listener cancelled"
}

// ListenerStopped is returned when StopListening is called externally.
// Mirrors pywa's ListenerStopped exception.
type ListenerStopped struct {
	Reason string
}

// Error implements the error interface.
func (e *ListenerStopped) Error() string {
	if e.Reason != "" {
		return "gowa: listener stopped: " + e.Reason
	}
	return "gowa: listener stopped"
}

// ── Listener key ──────────────────────────────────────────────────────────────

// ListenerKey uniquely identifies a pending listener.
// Mirrors pywa's UserUpdateListenerIdentifier.
//
// Fields:
//   - SenderWAID:  the WA ID of the user whose next update we are waiting for.
//   - RecipientID: the business phone number ID receiving the update.
type ListenerKey struct {
	SenderWAID  string
	RecipientID string
}

// ── Internal listener state ───────────────────────────────────────────────────

// listener is an internal object holding the pending state for one Listen call.
type listener struct {
	done      chan struct{}
	result    *Message
	exception error
	once      sync.Once
}

// resolve delivers a result to the waiting goroutine (called at most once).
func (l *listener) resolve(msg *Message) {
	l.once.Do(func() {
		l.result = msg
		close(l.done)
	})
}

// reject delivers an error to the waiting goroutine (called at most once).
func (l *listener) reject(err error) {
	l.once.Do(func() {
		l.exception = err
		close(l.done)
	})
}

// ── ListenOptions ─────────────────────────────────────────────────────────────

// ListenOptions configures a Listen call.
// Mirrors the parameters of pywa's WhatsApp.listen().
//
// Fields:
//   - SenderWAID:   WA ID of the user to wait for (required).
//   - RecipientID:  phone number ID receiving the update (uses Config.PhoneID if empty).
//   - Filters:      passes the update only when all filters return true (optional).
//   - Cancelers:    cancels the listener (returns ListenerCanceled) when any filter matches (optional).
//   - Timeout:      maximum wait duration; 0 means no timeout (optional).
type ListenOptions struct {
	SenderWAID  string
	RecipientID string
	Filters     Filter[*Message]
	Cancelers   Filter[*Message]
	Timeout     time.Duration
}

// ── Listen ────────────────────────────────────────────────────────────────────

// Listen blocks the calling goroutine until a matching *Message arrives from
// the specified sender, or until the listener times out / is cancelled / stopped.
//
// This is Go's equivalent of pywa's wa.listen().  Call it from inside a handler
// goroutine (each handler dispatch runs in its own goroutine via go dispatch()).
//
// Parameters:
//   - opts: ListenOptions specifying who to wait for, filters, and timeout.
//
// Returns:
//   - *Message: the first matching message.
//   - error:    *ListenerTimeout | *ListenerCanceled | *ListenerStopped | stdlib error.
//
// Example:
//
//	wa.OnMessage(func(wa *gowa.WhatsApp, msg *gowa.Message) {
//	    msg.Reply("What's your name?")
//	    reply, err := wa.Listen(gowa.ListenOptions{
//	        SenderWAID: msg.From.WAID,
//	        Filters:    gowa.FilterText,
//	        Timeout:    30 * time.Second,
//	    })
//	    if err != nil {
//	        msg.Reply("You took too long!")
//	        return
//	    }
//	    msg.Reply("Hello, " + *reply.Text + "!")
//	}, gowa.FilterText)
func (wa *WhatsApp) Listen(opts ListenOptions) (*Message, error) {
	if err := wa.requireWebhook(); err != nil {
		return nil, err
	}
	recipientID := opts.RecipientID
	if recipientID == "" {
		recipientID = wa.phoneID
	}
	if opts.SenderWAID == "" {
		return nil, fmt.Errorf("gowa: Listen requires SenderWAID")
	}

	key := ListenerKey{SenderWAID: opts.SenderWAID, RecipientID: recipientID}
	l := &listener{done: make(chan struct{})}

	wa.mu.Lock()
	if wa.listeners == nil {
		wa.listeners = map[ListenerKey]*listenerEntry{}
	}
	wa.listeners[key] = &listenerEntry{
		l:         l,
		filters:   opts.Filters,
		cancelers: opts.Cancelers,
	}
	wa.mu.Unlock()

	defer func() {
		wa.mu.Lock()
		delete(wa.listeners, key)
		wa.mu.Unlock()
	}()

	if opts.Timeout > 0 {
		timer := time.NewTimer(opts.Timeout)
		defer timer.Stop()
		select {
		case <-l.done:
		case <-timer.C:
			return nil, &ListenerTimeout{Duration: opts.Timeout}
		}
	} else {
		<-l.done
	}

	if l.exception != nil {
		return nil, l.exception
	}
	return l.result, nil
}

// StopListening cancels an active listener for the given sender.
// Mirrors pywa's WhatsApp.stop_listening().
//
// Parameters:
//   - senderWAID:  the sender WA ID of the listener to cancel.
//   - recipientID: the phone number ID (uses Config.PhoneID if empty).
//   - reason:      optional human-readable reason included in ListenerStopped.
//
// Returns:
//   - error if no such listener exists.
func (wa *WhatsApp) StopListening(senderWAID, recipientID, reason string) error {
	if recipientID == "" {
		recipientID = wa.phoneID
	}
	key := ListenerKey{SenderWAID: senderWAID, RecipientID: recipientID}

	wa.mu.Lock()
	entry, ok := wa.listeners[key]
	wa.mu.Unlock()

	if !ok {
		return fmt.Errorf("gowa: no active listener for sender=%s recipient=%s", senderWAID, recipientID)
	}
	entry.l.reject(&ListenerStopped{Reason: reason})
	return nil
}

// ── listenerEntry stores listener + its filters ───────────────────────────────

// listenerEntry pairs a listener with its filter predicates.
type listenerEntry struct {
	l         *listener
	filters   Filter[*Message]
	cancelers Filter[*Message]
}

// tryDeliver attempts to deliver msg to the listener.
// Called by dispatchMessage when a message arrives from the sender.
//
// Parameters:
//   - wa:  the active client.
//   - msg: the incoming message.
//
// Returns:
//   - true if the listener consumed the message (normal handlers should not fire).
func (e *listenerEntry) tryDeliver(wa *WhatsApp, msg *Message) bool {
	// Check cancelers first
	if e.cancelers != nil && e.cancelers(wa, msg) {
		e.l.reject(&ListenerCanceled{Update: msg})
		return true
	}
	// Check filters
	if e.filters == nil || e.filters(wa, msg) {
		e.l.resolve(msg)
		return true
	}
	return false
}

// ── Integration hook inside dispatchMessage ───────────────────────────────────
// notifyListeners is called from dispatchMessage (webhook.go) before dispatching
// to registered handlers.  It returns true if a listener consumed the message —
// in that case normal handler dispatch is skipped (mirrors pywa behaviour).
//
// Parameters:
//   - msg: the freshly parsed *Message.
//
// Returns:
//   - true if a listener consumed the message.
func (wa *WhatsApp) notifyListeners(msg *Message) bool {
	if len(wa.listeners) == 0 {
		return false
	}
	key := ListenerKey{
		SenderWAID:  msg.From.WAID,
		RecipientID: msg.Metadata.PhoneNumberID,
	}
	wa.mu.RLock()
	entry, ok := wa.listeners[key]
	wa.mu.RUnlock()

	if !ok {
		return false
	}
	return entry.tryDeliver(wa, msg)
}
