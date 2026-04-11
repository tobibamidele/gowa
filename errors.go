// Decorator-based handled registration (Python's @wa.on_message) is replaced in Go
// with functional registration via wa.OnMessage(handler, filters...) and
// wa.AddHandlers(...) - a clean, idiomatic Go equivalent that carries the same power

package gowa

import (
	"fmt"
	"net/http"
)

// WhatsAppError represents a structured error returned by the WhatsApp Cloud API
// or an error embedded inside an incoming webhook update.
//
// Reference:
//   - https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
//   - https://developers.facebook.com/docs/whatsapp/flows/reference/error-codes
//
// Fields:
//   - Code:        The Meta error code.
//   - Message:     The human-readable error message.
//   - Details:     Extra detail string (from error_data.details, optional).
//   - FBTraceID:   Facebook trace ID for debugging (optional).
//   - Href:        Link to relevant documentation (optional).
//   - Subcode:     Error sub-code (optional).
//   - Type:        Error type string (optional).
//   - IsTransient: Whether the error is transient and may succeed on retry (optional).
//   - UserTitle:   End-user-facing title (optional).
//   - UserMsg:     End-user-facing message (optional).
//   - StatusCode:  HTTP status code of the response that produced this error (0 if from webhook).
type WhatsAppError struct {
	Code        int
	Message     string
	Details     string
	FBTraceID   string
	Href        string
	Subcode     int
	Type        string
	IsTransient bool
	UserTitle   string
	UserMsg     string
	StatusCode  int
}

// Error implements the built-in error interface.
// Returns a formatted string: "WhatsAppError(code=N): message [details]"
func (e *WhatsAppError) Error() string {
	s := fmt.Sprintf("WhatsAppError(code=%d): %s", e.Code, e.Message)
	if e.Details != "" {
		s += " [" + e.Details + "]"
	}
	return s
}

// whatsAppErrorFromMap constructs a *WhatsAppError from the JSON map inside
// {"error": {...}} that the Graph API returns on failure.
//
// Parameters:
//   - errMap: the "error" sub-object from the API JSON response.
//   - resp:   the http.Response that contained the error (may be nil).
//
// Returns:
//   - *WhatsAppError decoded from the map; never nil.
func whatsAppErrorFromMap(errMap map[string]any, resp *http.Response) *WhatsAppError {
	e := &WhatsAppError{}
	if v, ok := errMap["code"]; ok {
		e.Code = int(toFloat64(v))
	}
	if v, ok := errMap["message"]; ok {
		e.Message = fmt.Sprint(v)
	}
	if errData, ok := errMap["error_data"].(map[string]any); ok {
		if d, ok := errData["details"]; ok {
			e.Details = fmt.Sprint(d)
		}
	}
	if v, ok := errMap["fbtrace_id"]; ok {
		e.FBTraceID = fmt.Sprint(v)
	}
	if v, ok := errMap["href"]; ok {
		e.Href = fmt.Sprint(v)
	}
	if v, ok := errMap["error_subcode"]; ok {
		e.Subcode = int(toFloat64(v))
	}
	if v, ok := errMap["type"]; ok {
		e.Type = fmt.Sprint(v)
	}
	if v, ok := errMap["is_transient"]; ok {
		e.IsTransient, _ = v.(bool)
	}
	if v, ok := errMap["error_user_title"]; ok {
		e.UserTitle = fmt.Sprint(v)
	}
	if v, ok := errMap["error_user_msg"]; ok {
		e.UserMsg = fmt.Sprint(v)
	}
	if resp != nil {
		e.StatusCode = resp.StatusCode
	}
	return e
}

// ── Named error sub-types ─────────────────────────────────────────────────────
// pywa defines many named error classes (AuthException, ThrottlingError, etc.)
// each mapped to specific Meta error codes.  In Go we embed WhatsAppError and
// expose the code via the ErrorKind enum so callers can switch on it cleanly.

// ErrorKind classifies a WhatsAppError into a broad category matching pywa's
// named exception hierarchy.
type ErrorKind int

const (
	ErrorKindGeneral          ErrorKind = iota
	ErrorKindAuth                       // code 190
	ErrorKindRateLimit                  // code 4, 130429, 131048, 131056
	ErrorKindServiceUnavail             // code 1, 2, 3, 130472
	ErrorKindInvalidParameter           // code 100
	ErrorKindPermission                 // code 10, 200-299
	ErrorKindPaymentIssue               // code 131042
	ErrorKindMessageTooBig              // code 131009
	ErrorKindInvalidFormat              // code 131016
	ErrorKindFlowBlocked                // code 131043, 131044
	ErrorKindFlowThrottle               // code 131045
	ErrorKindFlowError                  // code 132000+
)

// Kind returns the ErrorKind for the given error code.
//
// Parameters:
//   - code: Meta error code from WhatsAppError.Code.
//
// Returns:
//   - the matching ErrorKind constant.
func Kind(code int) ErrorKind {
	switch {
	case code == 190:
		return ErrorKindAuth
	case code == 4 || code == 130429 || code == 131048 || code == 131056:
		return ErrorKindRateLimit
	case code == 1 || code == 2 || code == 3 || code == 130472:
		return ErrorKindServiceUnavail
	case code == 100:
		return ErrorKindInvalidParameter
	case code == 10 || (code >= 200 && code <= 299):
		return ErrorKindPermission
	case code == 131042:
		return ErrorKindPaymentIssue
	case code == 131009:
		return ErrorKindMessageTooBig
	case code == 131016:
		return ErrorKindInvalidFormat
	case code == 131043 || code == 131044:
		return ErrorKindFlowBlocked
	case code == 131045:
		return ErrorKindFlowThrottle
	case code >= 132000:
		return ErrorKindFlowError
	default:
		return ErrorKindGeneral
	}
}
