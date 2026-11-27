package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	http "github.com/saucesteals/fhttp"
)

func (a *authenticator) duoSMS(ctx context.Context, p duoPrompt) (string, error) {
	allowed := false
	for _, f := range a.duoFactors {
		if f.Type == "sms_otp" && f.DeviceID == a.duoSMSDevice {
			allowed = true
		}
	}
	if !allowed {
		return "", &DuoMethodsRequired{Methods: a.duoFactors}
	}
	if a.requestDuoPasscode == nil {
		return "", errors.New("SMS authentication requires a code input callback")
	}
	send := func(path string, fields map[string]string) (*page, error) {
		fields["authkey"] = p.authKey
		fields["pkey"] = a.duoSMSDevice
		body, err := json.Marshal(fields)
		if err != nil {
			return nil, err
		}
		return a.do(ctx, navigation{method: http.MethodPost, url: p.base + "/auth/factors/sms_otp/" + path, body: body, contentType: "application/json", referer: p.page.URL, duoTraceGroup: p.traceGroup})
	}
	response, err := send("initialize", map[string]string{})
	if err != nil {
		return "", errors.New("Duo SMS request failed")
	}
	var started struct {
		Stat     string `json:"stat"`
		Response struct {
			Result string `json:"result"`
		} `json:"response"`
	}
	if response.Status != 200 || json.Unmarshal(response.Body, &started) != nil || started.Stat != "OK" || started.Response.Result != "SUCCESS" {
		return "", errors.New("Duo could not send an SMS code")
	}
	code, err := a.requestDuoPasscode()
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(code) == "" {
		return "", errors.New("SMS code is required")
	}
	response, err = send("auth", map[string]string{"code": strings.TrimSpace(code)})
	if err != nil {
		return "", errors.New("Duo SMS verification failed")
	}
	if err = validateDuoFactor(response.Status, response.Body); err != nil {
		if errors.Is(err, ErrBypassRejected) {
			return "", ErrDuoSMSRejected
		}
		return "", err
	}
	return jsonStrings(response.Body)["oidc_code"], nil
}
