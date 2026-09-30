package libcal

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ResponseError describes a rejected LibCal HTTP request without exposing the
// response body, which may contain user or session state.
type ResponseError struct {
	Operation string
	Status    int
	BodyBytes int
	Cause     error
}

// Error provides a stable machine-readable code and a safe explanation.
type Error struct {
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string     { return e.Message }
func (e *Error) ErrorCode() string { return e.Code }
func (e *Error) Unwrap() error     { return e.Cause }
func (e *Error) Is(target error) bool {
	other, ok := target.(*Error)
	return ok && other.Code == e.Code
}

func libcalError(message string) *Error {
	message = safeMessage(textFromHTML([]byte(message)))
	lower := strings.ToLower(message)
	code := "libcal_rejected"
	switch {
	case strings.Contains(lower, "dates have become unavailable"), strings.Contains(lower, "no longer available"):
		code = "libcal_slot_unavailable"
		message = "LibCal: the selected dates have become unavailable; refresh availability and choose another slot"
	case strings.Contains(lower, "checksum"):
		code = "libcal_stale_slot"
		message = "LibCal: the availability checksum is stale; refresh availability before retrying"
	case strings.Contains(lower, "invalid id"), strings.Contains(lower, "expired"), strings.Contains(lower, "invalid session"):
		code = "libcal_checkout_expired"
		message = "LibCal: the checkout session is invalid or expired; start a new booking"
	case strings.Contains(lower, "authentication"), strings.Contains(lower, "sign in"), strings.Contains(lower, "log in"):
		code = "libcal_authentication_required"
		message = ErrAuthenticationRequired.Error()
	case strings.Contains(lower, "limit"), strings.Contains(lower, "maximum"):
		code = "libcal_booking_limit"
	case strings.Contains(lower, "required"), strings.Contains(lower, "field"):
		code = "libcal_invalid_details"
	}
	// Do not echo secret-bearing diagnostic responses. Known failure classes
	// above use fixed messages; other short server validation messages remain
	// useful to both humans and agents.
	if message == "" || strings.Contains(strings.ToLower(message), "cookie") ||
		strings.Contains(strings.ToLower(message), "token") ||
		strings.Contains(strings.ToLower(message), "password") {
		message = "LibCal rejected the request"
	}
	return &Error{Code: code, Message: message}
}

func responseError(operation string, status int, data []byte) *ResponseError {
	message := ""
	var wire struct {
		Error string `json:"error"`
	}
	var plain string
	switch {
	case json.Unmarshal(data, &wire) == nil && wire.Error != "":
		message = wire.Error
	case json.Unmarshal(data, &plain) == nil:
		message = plain
	case len(data) <= 1024 && !strings.Contains(strings.ToLower(string(data)), "<script"):
		message = string(data)
	}
	cause := libcalError(message)
	if status == 401 || status == 403 {
		cause = ErrAuthenticationRequired
	} else if status == 429 {
		cause = &Error{Code: "libcal_rate_limited", Message: "LibCal rate limited the request; wait before retrying"}
	} else if status >= 500 {
		cause = &Error{Code: "libcal_unavailable", Message: "LibCal is temporarily unavailable"}
	}
	return &ResponseError{Operation: operation, Status: status, BodyBytes: len(data), Cause: cause}
}

func (e *ResponseError) ErrorCode() string {
	if coded, ok := e.Cause.(interface{ ErrorCode() string }); ok {
		return coded.ErrorCode()
	}
	return "libcal_http_error"
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("LibCal %s: HTTP %d: %v", e.Operation, e.Status, e.Cause)
}

func (e *ResponseError) Unwrap() error { return e.Cause }
