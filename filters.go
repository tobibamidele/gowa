package gowa

// ────────────────────────────────────────────────────────────────────────────────
// Filter system
// Python's @wa.on_message(filters.text) decorator pattern becomes in Go:
//
//   wa.OnMessage(handler, gowa.FilterText)
//   wa.OnMessage(handler, gowa.And(gowa.FilterText, gowa.FilterFromWAID("1234567890")))
//
// A Filter is simply a function from (client, update) → bool.  Compound filters
// are built with And, Or, Not.
// ────────────────────────────────────────────────────────────────────────────────

// Filter is a predicate that decides whether a handler should be called for a
// given update.  The first argument is the WhatsApp client (for stateful filters
// that need API access), the second is the raw update value.
//
// Mirrors pywa's Filter callable objects.
type Filter[T any] func(wa *WhatsApp, update T) bool

// Always returns a Filter that always passes.
//
// Returns:
//   - Filter[T] that always returns true.
func Always[T any]() Filter[T] {
	return func(_ *WhatsApp, _ T) bool { return true }
}

// Never returns a Filter that always blocks.
//
// Returns:
//   - Filter[T] that always returns false.
func Never[T any]() Filter[T] {
	return func(_ *WhatsApp, _ T) bool { return false }
}

// And composes multiple filters with logical AND.
// Evaluation is short-circuit: returns false as soon as one filter fails.
//
// Parameters:
//   - filters: one or more Filter[T] to combine.
//
// Returns:
//   - A single Filter[T] that returns true iff all sub-filters return true.
func And[T any](filters ...Filter[T]) Filter[T] {
	return func(wa *WhatsApp, u T) bool {
		for _, f := range filters {
			if f != nil && !f(wa, u) {
				return false
			}
		}
		return true
	}
}

// Or composes multiple filters with logical OR.
// Evaluation is short-circuit: returns true as soon as one filter passes.
//
// Parameters:
//   - filters: one or more Filter[T] to combine.
//
// Returns:
//   - A single Filter[T] that returns true if any sub-filter returns true.
func Or[T any](filters ...Filter[T]) Filter[T] {
	return func(wa *WhatsApp, u T) bool {
		for _, f := range filters {
			if f != nil && f(wa, u) {
				return true
			}
		}
		return false
	}
}

// Not inverts a filter.
//
// Parameters:
//   - f: the Filter[T] to negate.
//
// Returns:
//   - A Filter[T] that returns true when f returns false.
func Not[T any](f Filter[T]) Filter[T] {
	return func(wa *WhatsApp, u T) bool {
		if f == nil {
			return false
		}
		return !f(wa, u)
	}
}

// ── Message filters ───────────────────────────────────────────────────────────

// FilterText matches messages that contain a text body.
//
// Equivalent to pywa's filters.text
var FilterText Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeText && m.Text != nil && *m.Text != ""
}

// FilterImage matches messages that contain an image.
var FilterImage Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeImage && m.Image != nil
}

// FilterVideo matches messages that contain a video.
var FilterVideo Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeVideo && m.Video != nil
}

// FilterAudio matches messages that contain audio (includes voice notes).
var FilterAudio Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeAudio && m.Audio != nil
}

// FilterVoice matches messages that contain a voice note specifically.
var FilterVoice Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Audio != nil && m.Audio.Voice
}

// FilterDocument matches messages that contain a document.
var FilterDocument Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeDocument && m.Document != nil
}

// FilterSticker matches messages that contain a sticker.
var FilterSticker Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeSticker && m.Sticker != nil
}

// FilterLocation matches messages that contain a location.
var FilterLocation Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeLocation && m.Location != nil
}

// FilterContacts matches messages that contain contact cards.
var FilterContacts Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeContacts && len(m.Contacts) > 0
}

// FilterReaction matches messages that are emoji reactions.
var FilterReaction Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Type == MessageTypeReaction && m.Reaction != nil
}

// FilterReply matches messages that are replies to another message.
var FilterReply Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.IsReply()
}

// FilterForwarded matches messages that were forwarded.
var FilterForwarded Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.Forwarded
}

// FilterMedia matches any message that has a media attachment.
var FilterMedia Filter[*Message] = func(_ *WhatsApp, m *Message) bool {
	return m.HasMedia()
}

// FilterFromWAID returns a filter that matches messages from a specific WhatsApp ID.
//
// Parameters:
//   - waID: the sender's phone number / WhatsApp ID to match.
//
// Returns:
//   - Filter[*Message] that returns true when m.From.WAID == waID.
func FilterFromWAID(waID string) Filter[*Message] {
	return func(_ *WhatsApp, m *Message) bool {
		return m.From.WAID == waID
	}
}

// FilterTextContains returns a filter that matches text messages whose body
// contains the given substring (case-sensitive).
//
// Parameters:
//   - substr: substring to search for.
//
// Returns:
//   - Filter[*Message] performing a substring check.
func FilterTextContains(substr string) Filter[*Message] {
	return func(_ *WhatsApp, m *Message) bool {
		if m.Text == nil {
			return false
		}
		return contains(*m.Text, substr)
	}
}

// FilterTextPrefix returns a filter matching text messages that start with prefix.
//
// Parameters:
//   - prefix: prefix string to check.
//
// Returns:
//   - Filter[*Message] performing a prefix check.
func FilterTextPrefix(prefix string) Filter[*Message] {
	return func(_ *WhatsApp, m *Message) bool {
		if m.Text == nil {
			return false
		}
		return hasPrefix(*m.Text, prefix)
	}
}

// ── CallbackButton filters ────────────────────────────────────────────────────

// FilterCallbackData returns a filter matching CallbackButton/Selection updates
// whose Data equals the given string.
//
// Parameters:
//   - data: the exact callback data string to match.
//
// Returns:
//   - Filter[*CallbackButton] that returns true when cb.Data == data.
func FilterCallbackData(data string) Filter[*CallbackButton] {
	return func(_ *WhatsApp, cb *CallbackButton) bool {
		return cb.Data == data
	}
}

// FilterCallbackPrefix returns a filter matching CallbackButton updates
// whose Data has the given prefix.
//
// Parameters:
//   - prefix: the prefix string to match.
//
// Returns:
//   - Filter[*CallbackButton] performing a prefix check.
func FilterCallbackPrefix(prefix string) Filter[*CallbackButton] {
	return func(_ *WhatsApp, cb *CallbackButton) bool {
		return hasPrefix(cb.Data, prefix)
	}
}

// ── MessageStatus filters ─────────────────────────────────────────────────────

// FilterStatusSent matches message status updates where the status is "sent".
var FilterStatusSent Filter[*MessageStatus] = func(_ *WhatsApp, s *MessageStatus) bool {
	return s.Status == MessageStatusSent
}

// FilterStatusDelivered matches message status updates where the status is "delivered".
var FilterStatusDelivered Filter[*MessageStatus] = func(_ *WhatsApp, s *MessageStatus) bool {
	return s.Status == MessageStatusDelivered
}

// FilterStatusRead matches message status updates where the status is "read".
var FilterStatusRead Filter[*MessageStatus] = func(_ *WhatsApp, s *MessageStatus) bool {
	return s.Status == MessageStatusRead
}

// FilterStatusFailed matches message status updates where the status is "failed".
var FilterStatusFailed Filter[*MessageStatus] = func(_ *WhatsApp, s *MessageStatus) bool {
	return s.Status == MessageStatusFailed
}

// ── small helpers ─────────────────────────────────────────────────────────────
// We purposefully avoid importing strings to minimise dependencies.

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexSubstr(s, substr) >= 0
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func indexSubstr(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
