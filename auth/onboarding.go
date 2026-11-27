package auth

import "strings"

type SignInRequired struct {
	Missing []string
}

func (e *SignInRequired) Error() string {
	return "USC sign-in requires " + strings.Join(e.Missing, ", ")
}

func (e *SignInRequired) ErrorCode() string { return "usc_sign_in_required" }
func (e *SignInRequired) Unwrap() error     { return ErrCredentialsRequired }
