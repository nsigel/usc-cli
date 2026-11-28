package auth

import (
	"context"
	"errors"
)

type Options struct {
	DuoDeviceName      string
	DuoSMSDevice       string
	RequestDuoPasscode func() (string, error)
	DuoPasscode        string
	DuoPushDevice      string
	DuoPushStarted     func(string)
	Credentials        Credentials

	CredentialsFile string

	SessionFile string
}

func (o Options) ResolveCredentials() (Credentials, error) {
	credentials := o.Credentials
	if o.CredentialsFile == "" {
		return credentials, nil
	}
	saved, err := LoadCredentials(o.CredentialsFile)
	if err != nil {
		return Credentials{}, err
	}
	if credentials.DuoDeviceID == "" {
		credentials.DuoDeviceID = saved.DuoDeviceID
	}
	if credentials.Username == "" {
		credentials.Username = saved.Username
	}
	if credentials.Password == "" {
		credentials.Password = saved.Password
	}
	if credentials.BypassCode == "" && o.Credentials.DuoTOTPSecret == "" {
		credentials.BypassCode = saved.BypassCode
	}
	if credentials.DuoTOTPSecret == "" && o.Credentials.BypassCode == "" {
		credentials.DuoTOTPSecret = saved.DuoTOTPSecret
	}
	return credentials, nil
}

type SiteError struct {
	Site Site
	Err  error
}

func (e *SiteError) Error() string { return string(e.Site) + ": " + e.Err.Error() }
func (e *SiteError) Unwrap() error { return e.Err }

func ForSite(name Site, err error) error {
	var attributed *SiteError
	if errors.As(err, &attributed) {
		return err
	}
	for _, failure := range []error{ErrCredentialsRequired, ErrPasswordRejected, ErrBypassRejected, ErrDuoPasscodeRejected, ErrDuoSMSRejected, ErrDuoPushRejected, ErrSessionInvalid} {
		if errors.Is(err, failure) {
			return &SiteError{Site: name, Err: err}
		}
	}
	return err
}

func Open(ctx context.Context, name Site, options Options) (*Session, error) {
	target, err := loginURL(name)
	if err != nil {
		return nil, err
	}
	session, err := OpenSession(options, CookiePolicy{})
	if err != nil {
		return nil, err
	}
	response, err := session.Authenticate(ctx, target, "")
	if response != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, ForSite(name, err)
	}
	return session, nil
}
