package cli

import (
	"errors"

	"github.com/nsigel/usc-cli/internal/auth"
)

type AgentError struct {
	Code    string
	Message string
	Action  string
	Err     error
}

func (e *AgentError) Error() string { return e.Message }
func (e *AgentError) Unwrap() error { return e.Err }

func authenticationError(profile string, err error) error {
	switch {
	case errors.Is(err, auth.ErrBypassRejected):
		return &AgentError{
			Code:    "duo_bypass_invalid",
			Message: "the saved Duo bypass code is invalid or expired",
			Action:  "usc bypass <new-code>",
			Err:     err,
		}
	case errors.Is(err, auth.ErrCredentialsRequired):
		return &AgentError{
			Code:    "credentials_required",
			Message: "profile " + profile + " needs USC credentials",
			Action:  "usc login",
			Err:     err,
		}
	default:
		return err
	}
}

func ErrorPayload(err error) map[string]any {
	payload := map[string]any{"error": err.Error()}
	var agentError *AgentError
	if errors.As(err, &agentError) {
		payload["code"] = agentError.Code
		payload["action"] = agentError.Action
	}
	return payload
}
