package gowa

import "encoding/json"

// ────────────────────────────────────────────────────────────────────────────────
// Shared utilities
// ────────────────────────────────────────────────────────────────────────────────

// jsonMarshal is the single JSON serialisation entry-point used across the package.
// Having one alias makes it trivial to swap implementations (e.g. sonic/jsoniter)
// if performance ever becomes a concern.
//
// Parameters:
//   - v: any value to serialise.
//
// Returns:
//   - []byte of the JSON representation.
//   - error if v cannot be serialised.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// jsonUnmarshal is the single JSON deserialisation entry-point.
//
// Parameters:
//   - data: raw JSON bytes.
//   - v:    pointer to the target value.
//
// Returns:
//   - error if the data cannot be decoded into v.
func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// listeners field — added to WhatsApp struct via the listeners.go file.
// We declare it here as a type alias so the compiler can see the field
// when compiling listeners.go and client.go together.
// The actual field is injected directly into the WhatsApp struct below.
// NOTE: Go does not support "partial structs", so the listeners map is declared
// as a field directly in the WhatsApp struct in client.go.
// This file just documents the pattern.
//
// The WhatsApp struct (client.go) carries:
//
//	listeners map[ListenerKey]*listenerEntry
//
// which is initialised lazily in Listen().
