package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/internal/dom"
	http "github.com/saucesteals/fhttp"
)

type DuoDevice struct {
	Name             string `json:"name,omitempty"`
	ID               string `json:"id"`
	CredentialsSaved bool   `json:"credentials_saved"`
	Secret           string `json:"-"`
}

type DuoDeviceInfo struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Name    string `json:"name,omitempty"`
	Model   string `json:"model,omitempty"`
	Number  string `json:"number,omitempty"`
	Current bool   `json:"current,omitempty"`
}

type duoPortalData struct {
	XSRF   string `json:"xsrf_token"`
	Phones []struct {
		Key          string `json:"key"`
		Name         string `json:"name"`
		Model        string `json:"model"`
		Number       string `json:"number"`
		PrettyNumber string `json:"pretty_number"`
	} `json:"phones"`
	WebAuthn []struct {
		Key      string `json:"key"`
		Name     string `json:"name"`
		Metadata struct {
			Browser string `json:"browser"`
			OS      string `json:"os"`
		} `json:"credential_metadata"`
	} `json:"webauthn_credentials"`
}

var duoActivationHost = regexp.MustCompile(`^api-[a-zA-Z0-9]+\.duosecurity\.com$`)

func ListDuoDevices(ctx context.Context, options Options) ([]DuoDeviceInfo, error) {
	s, err := OpenSession(options, CookiePolicy{})
	if err != nil {
		return nil, err
	}
	_, portal, err := duoPortal(ctx, s)
	if err != nil {
		return nil, err
	}
	devices := make([]DuoDeviceInfo, 0, len(portal.Phones)+len(portal.WebAuthn))
	for _, phone := range portal.Phones {
		number := phone.PrettyNumber
		if number == "" {
			number = phone.Number
		}
		devices = append(devices, DuoDeviceInfo{
			ID:      phone.Key,
			Type:    "phone",
			Name:    strings.TrimSpace(phone.Name),
			Model:   strings.TrimSpace(phone.Model),
			Number:  strings.TrimSpace(number),
			Current: s.credentials.DuoDeviceID != "" && phone.Key == s.credentials.DuoDeviceID,
		})
	}
	for _, cred := range portal.WebAuthn {
		model := strings.TrimSpace(cred.Metadata.OS)
		if browser := strings.TrimSpace(cred.Metadata.Browser); browser != "" {
			if model != "" {
				model += " (" + browser + ")"
			} else {
				model = browser
			}
		}
		devices = append(devices, DuoDeviceInfo{
			ID:    cred.Key,
			Type:  "webauthn",
			Name:  strings.TrimSpace(cred.Name),
			Model: model,
		})
	}
	_ = s.save()
	return devices, nil
}

func CreateDuoDevice(ctx context.Context, options Options) (*DuoDevice, error) {
	s, err := OpenSession(options, CookiePolicy{})
	if err != nil {
		return nil, err
	}
	if s.credentials.DuoTOTPSecret != "" {
		return nil, errors.New("a Duo TOTP credential is already configured; delete that enrollment before creating another")
	}
	base, portal, err := duoPortal(ctx, s)
	if err != nil {
		return nil, err
	}
	token := portal.XSRF
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/frame/device-management/api/mobile_phone/activate", strings.NewReader(`{"phone_number":"","platform":"Generic Smartphone"}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Xsrftoken", token)
	req.Header.Set("Origin", base)
	req.Header.Set("Referer", base+"/frame/device-management/portal")
	var envelope struct {
		Stat     string          `json:"stat"`
		Response json.RawMessage `json:"response"`
		Phone    struct {
			Key string `json:"key"`
		} `json:"phone"`
		Activation struct {
			Code string `json:"actcode_hostname"`
		} `json:"activation_info"`
	}
	if err := duoDeviceRequest(s, req, &envelope); err != nil {
		return nil, fmt.Errorf("device creation outcome may be unknown; inspect USC device management before retrying: %w", err)
	}
	if len(envelope.Response) > 0 {
		if err := json.Unmarshal(envelope.Response, &envelope); err != nil {
			return nil, errors.New("unsupported Duo enrollment response; inspect USC device management before retrying")
		}
	}
	result := &DuoDevice{ID: envelope.Phone.Key}
	if result.ID == "" {
		return nil, errors.New("Duo returned no device ID; inspect USC device management before retrying")
	}
	secret, err := activateDuoDevice(ctx, s, envelope.Activation.Code)
	if err != nil {
		return result, fmt.Errorf("device %s created but activation failed; remove it in USC device management before retrying: %w", result.ID, err)
	}
	result.Secret = secret
	if options.CredentialsFile != "" {
		credentials := s.credentials
		credentials.DuoTOTPSecret = secret
		credentials.DuoDeviceID = result.ID
		credentials.BypassCode = ""
		if err := SaveCredentials(options.CredentialsFile, credentials); err != nil {
			return result, fmt.Errorf("device %s activated but credentials were not saved; retain the returned secret or remove the device before retrying: %w", result.ID, err)
		}
		result.CredentialsSaved = true
	}
	if name := strings.TrimSpace(options.DuoDeviceName); name != "" {
		body, err := json.Marshal(map[string]string{"name": name})
		if err != nil {
			return result, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, base+"/frame/device-management/api/phone/"+url.PathEscape(result.ID), strings.NewReader(string(body)))
		if err != nil {
			return result, err
		}
		req.Header.Set("X-Xsrftoken", token)
		req.Header.Set("Origin", base)
		req.Header.Set("Referer", base+"/frame/device-management/portal")
		req.Header.Set("Content-Type", "application/json")
		var renamed struct {
			Stat string `json:"stat"`
		}
		if err := duoDeviceRequest(s, req, &renamed); err != nil {
			return result, fmt.Errorf("device %s activated but naming failed; do not create another device: %w", result.ID, err)
		}
		if renamed.Stat != "OK" {
			return result, fmt.Errorf("device %s activated but Duo did not confirm its name; do not create another device", result.ID)
		}
		result.Name = name
	}
	if err := s.save(); err != nil {
		return result, fmt.Errorf("device %s activated but session was not saved: %w", result.ID, err)
	}
	return result, nil
}

func duoDeviceRequest(s *Session, req *http.Request, out any) error {
	response, err := s.Do(req)

	if err != nil {
		return errors.New("Duo device request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Duo device request returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return errors.New("Duo device request returned invalid JSON")
	}
	return nil
}

func activateDuoDevice(ctx context.Context, s *Session, activation string) (string, error) {
	parts := strings.SplitN(activation, "-", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "", errors.New("invalid Duo activation credential")
	}
	decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil || !duoActivationHost.Match(decoded) {
		return "", errors.New("invalid Duo activation host")
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", err
	}
	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", err
	}

	// Emulate the current Duo Mobile for Android release enrolling from a
	// current Samsung tablet: the form fields, activation URL, and user agent
	// below match what Duo Mobile 4.126.0 (versionCode 4126000) sends when it
	// posts to /push/v2/activation/<code>. The security patch level reports the
	// current month, as an up-to-date Samsung device would.
	patchLevel := time.Now().UTC().Format("2006-01") + "-05"
	fields := url.Values{
		"pubkey": {string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public}))},
		"pkpush": {"rsa-sha512"}, "jailbroken": {"false"}, "architecture": {"aarch64"}, "region": {"US"},
		"app_id": {"com.duosecurity.duomobile"}, "full_disk_encryption": {"true"}, "passcode_status": {"true"},
		"app_version": {"4.126.0"}, "app_build_number": {"4126000"}, "version": {"16"},
		"manufacturer": {"samsung"}, "language": {"en"}, "security_patch_level": {patchLevel},
		"platform": {"Android"}, "model": {"Galaxy Tab S9"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://"+string(decoded)+"/push/v2/activation/"+url.PathEscape(parts[0])+"?customer_protocol=1", strings.NewReader(fields.Encode()))
	if err != nil {
		return "", errors.New("invalid Duo activation request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "DuoMobileApp/4.126.0 (Android 16; Galaxy Tab S9) okhttp3/4.12.0 ca_bundle/1.0 (ca_pinning=enabled)")
	var response struct {
		Stat     string `json:"stat"`
		Response struct {
			Secret string `json:"hotp_secret"`
			TOTP   bool   `json:"use_totp"`
		} `json:"response"`
	}
	if err := duoDeviceRequest(s, req, &response); err != nil {
		return "", err
	}
	if response.Stat != "OK" || !response.Response.TOTP || response.Response.Secret == "" {
		return "", errors.New("Duo did not provide a TOTP enrollment")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(response.Response.Secret)), nil
}

func duoPortal(ctx context.Context, s *Session) (string, duoPortalData, error) {
	page, err := s.authenticator.open(ctx, "https://account.usc.edu/2fa/enroll", s.credentials, nil)
	if err != nil {
		forgetRejected(s.options.CredentialsFile, err, s.credentials)
		return "", duoPortalData{}, ForSite(SiteAccount, err)
	}
	if page.URL.Scheme != "https" || page.URL.Host != "uw1.devicemanagement.duosecurity.com" {
		return "", duoPortalData{}, errors.New("Duo device management did not return a trusted portal")
	}
	var portal duoPortalData
	data := dom.RawText(dom.FindFirst(dom.Parse(page.Body), dom.ID("base-data")))
	if json.Unmarshal([]byte(html.UnescapeString(data)), &portal) != nil || portal.XSRF == "" {
		return "", duoPortalData{}, errors.New("Duo device management requires interactive verification or returned unsupported portal data")
	}
	base := "https://" + page.URL.Host
	return base, portal, nil
}

type DuoDeviceDeletion struct {
	Deleted bool `json:"deleted"`
}

func DeleteDuoDevice(ctx context.Context, options Options, id string) (*DuoDeviceDeletion, error) {
	if id == "" || strings.ContainsAny(id, "/?#") {
		return nil, errors.New("a valid device ID is required")
	}
	s, err := OpenSession(options, CookiePolicy{})
	if err != nil {
		return nil, err
	}
	base, portal, err := duoPortal(ctx, s)
	if err != nil {
		return nil, err
	}
	for _, cred := range portal.WebAuthn {
		if cred.Key == id {
			return nil, errors.New("WebAuthn credentials must be deleted in USC Duo device management")
		}
	}
	token := portal.XSRF
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/frame/device-management/api/phone/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Xsrftoken", token)
	req.Header.Set("Origin", base)
	req.Header.Set("Referer", base+"/frame/device-management/portal")
	req.Header.Set("Content-Type", "application/json")
	var response struct {
		Stat string `json:"stat"`
	}
	if err := duoDeviceRequest(s, req, &response); err != nil {
		return nil, err
	}
	if response.Stat != "OK" {
		return nil, errors.New("Duo did not confirm device deletion")
	}
	_ = s.save()
	result := &DuoDeviceDeletion{Deleted: true}
	if options.CredentialsFile != "" {
		saved, err := LoadCredentials(options.CredentialsFile)
		if err != nil {
			return result, err
		}
		if saved.DuoDeviceID == id {
			saved.DuoDeviceID, saved.DuoTOTPSecret = "", ""
			if err := SaveCredentials(options.CredentialsFile, saved); err != nil {
				return result, fmt.Errorf("device deleted but local credentials could not be cleared: %w", err)
			}
		}
	}
	return result, nil
}
