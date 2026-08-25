package cli

import (
	"errors"

	"github.com/nsigel/usc-cli/internal/auth"
)

func ErrorPayload(err error) map[string]any {
	payload := map[string]any{"error": err.Error()}
	switch {
	case errors.Is(err, auth.ErrBypassRejected):
		payload["code"] = "duo_bypass_invalid"
		payload["action"] = "usc bypass <new-code>"
	case errors.Is(err, auth.ErrCredentialsRequired):
		payload["code"] = "credentials_required"
		payload["action"] = "usc login"
	}
	return payload
}
