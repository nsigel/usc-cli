// Package usc provides shared error descriptions for USC service clients.
// Service implementations live in the auth, brightspace, classes, handshake,
// libcal, and browser subpackages.
package usc

import (
	"errors"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
	"github.com/nsigel/usc-cli/handshake"
	"github.com/nsigel/usc-cli/libcal"
	"github.com/nsigel/usc-cli/site"
)

// ErrorInfo is a JSON-ready error description with optional recovery guidance.
type ErrorInfo struct {
	Error  string `json:"error"`
	Code   string `json:"code,omitempty"`
	Action string `json:"action,omitempty"`
}

// DescribeError classifies existing errors without retrying or authenticating.
// The site name supplies context for shared auth errors; an empty name uses
// the default login command. A nil error returns an empty description.
func DescribeError(err error, name site.Name) ErrorInfo {
	if err == nil {
		return ErrorInfo{}
	}
	result := ErrorInfo{Error: err.Error()}
	var coded interface{ ErrorCode() string }
	if errors.As(err, &coded) {
		result.Code = coded.ErrorCode()
	}
	switch {
	case errors.Is(err, handshake.ErrSessionInvalid):
		result.Action = "usc auth login handshake"
	case errors.Is(err, libcal.ErrAuthenticationRequired):
		result.Action = "usc auth login libcal"
	case errors.Is(err, brightspace.ErrSessionInvalid):
		result.Action = "usc auth login"
	case errors.Is(err, auth.ErrCredentialsRequired), errors.Is(err, auth.ErrBypassRejected), errors.Is(err, auth.ErrSessionNotFound):
		result.Action = "usc auth login"
		if name == site.Handshake || name == site.LibCal {
			result.Action += " " + string(name)
		}
	}
	return result
}
