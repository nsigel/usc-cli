package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	http "github.com/saucesteals/fhttp"
)

type duoPrompt struct {
	page       *page
	base       string
	authKey    string
	traceGroup string
}

func (a *authenticator) duo(ctx context.Context, current *page, credentials Credentials) (*page, error) {
	prompt := duoPrompt{
		page:       current,
		base:       "https://" + current.URL.Host + current.URL.Path,
		authKey:    current.URL.Query().Get("authkey"),
		traceGroup: current.URL.Query().Get("req_trace_group"),
	}
	if prompt.authKey == "" || prompt.traceGroup == "" {
		return nil, errors.New("duo prompt is missing authkey or req_trace_group")
	}
	if err := a.duoPreflight(ctx, prompt); err != nil {
		return nil, err
	}
	if a.duoSMSDevice != "" {
		code, err := a.duoSMS(ctx, prompt)
		if err != nil {
			return nil, err
		}
		return a.duoFinalize(ctx, prompt, code)
	}
	if a.duoPushDevice != "" {
		code, err := a.duoPush(ctx, prompt)
		if err != nil {
			return nil, err
		}
		return a.duoFinalize(ctx, prompt, code)
	}
	factor, code := "bypass_code", credentials.BypassCode
	if a.requestDuoPasscode != nil {
		var err error
		code, err = a.requestDuoPasscode()
		if err != nil {
			return nil, err
		}
		factor = "mobile_otp"
	} else if a.duoPasscode != "" {
		factor, code = "mobile_otp", a.duoPasscode
		a.duoPasscode = ""
	} else if credentials.DuoTOTPSecret != "" {
		factor = "mobile_otp"
		var err error
		code, err = duoTOTP(credentials.DuoTOTPSecret, time.Now())
		if err != nil {
			return nil, err
		}
	}
	if code == "" {
		return nil, &DuoMethodsRequired{Methods: a.duoFactors}
	}
	oidcCode, err := a.duoSubmitPasscode(ctx, prompt, factor, code)
	if err != nil {
		return nil, err
	}
	return a.duoFinalize(ctx, prompt, oidcCode)
}

func (a *authenticator) duoPreflight(ctx context.Context, prompt duoPrompt) error {
	features, hints, err := duoClientDetails(a.duoClientHintUA)
	if err != nil {
		return err
	}
	payload, err := a.get(ctx, prompt.base+"/auth/payload?"+encodeFields([]field{
		{Name: "authkey", Value: prompt.authKey},
		{Name: "browser_features", Value: features},
		{Name: "is_ipad", Value: "false"},
		{Name: "client_hints", Value: hints},
	}), prompt.page.URL)
	if err != nil {
		return err
	}
	if payload.Status != http.StatusOK {
		return fmt.Errorf("duo auth/payload returned %d", payload.Status)
	}
	evaluation, err := a.do(ctx, navigation{
		url: prompt.base + "/pre_authn/evaluation?" + encodeFields([]field{
			{Name: "authkey", Value: prompt.authKey},
			{Name: "browser_features", Value: features},
			{Name: "local_trust_choice", Value: "trusted"},
		}),
		referer: prompt.page.URL, duoTraceGroup: prompt.traceGroup,
	})
	if err != nil {
		return err
	}
	if evaluation.Status != http.StatusOK {
		return fmt.Errorf("duo pre_authn/evaluation returned %d", evaluation.Status)
	}
	var data struct {
		Response struct {
			Context struct {
				Available struct {
					Factors []struct {
						Type   string `json:"factor_type"`
						Device struct {
							ID   string `json:"pkey"`
							Name string `json:"name"`
						} `json:"device_info"`
					} `json:"factors"`
				} `json:"available_unified_auth_factors"`
			} `json:"auth_factors_context"`
		} `json:"response"`
	}
	if json.Unmarshal(evaluation.Body, &data) != nil {
		return errors.New("invalid Duo method response")
	}
	a.duoFactors = nil
	for _, f := range data.Response.Context.Available.Factors {
		a.duoFactors = append(a.duoFactors, DuoMethod{Type: f.Type, DeviceID: f.Device.ID, Name: f.Device.Name})
	}
	return nil
}

func (a *authenticator) duoSubmitPasscode(ctx context.Context, prompt duoPrompt, factorName, passcode string) (string, error) {
	body, err := json.Marshal(map[string]string{"authkey": prompt.authKey, factorName: passcode})
	if err != nil {
		return "", err
	}
	factor, err := a.do(ctx, navigation{
		method: http.MethodPost, url: prompt.base + "/auth/factors/" + factorName,
		body: body, contentType: "application/json",
		referer: prompt.page.URL, duoTraceGroup: prompt.traceGroup,
	})
	if err != nil {
		return "", err
	}
	if err := validateDuoFactor(factor.Status, factor.Body); err != nil {
		if factorName == "mobile_otp" && errors.Is(err, ErrBypassRejected) {
			return "", ErrDuoPasscodeRejected
		}
		return "", err
	}
	return jsonStrings(factor.Body)["oidc_code"], nil
}

func (a *authenticator) duoFinalize(ctx context.Context, prompt duoPrompt, oidcCode string) (*page, error) {
	parameters := []field{{Name: "authkey", Value: prompt.authKey}}
	if oidcCode != "" {
		parameters = append(parameters, field{Name: "oidc_code", Value: oidcCode})
	}
	final, err := a.do(ctx, navigation{
		url:     prompt.base + "/auth/finalize_auth?" + encodeFields(parameters),
		referer: prompt.page.URL, duoTraceGroup: prompt.traceGroup,
	})
	if err != nil {
		return nil, err
	}
	if final.Status != http.StatusOK {
		return nil, fmt.Errorf("duo finalize_auth returned %d", final.Status)
	}
	exitURL := jsonStrings(final.Body)["url"]
	if exitURL == "" {
		return nil, errors.New("duo finalization did not return an exit URL")
	}
	return a.get(ctx, exitURL, prompt.page.URL)
}

var clientHintBrandPattern = regexp.MustCompile(`"([^"\\]+)";v="([^"\\]+)"`)

type duoBrand struct {
	Brand   string `json:"brand"`
	Version string `json:"version"`
}

type duoClientHints struct {
	Brands          []duoBrand `json:"brands"`
	FullVersionList []duoBrand `json:"fullVersionList"`
	Mobile          bool       `json:"mobile"`
	Platform        string     `json:"platform"`
	PlatformVersion string     `json:"platformVersion"`
	UAFullVersion   string     `json:"uaFullVersion"`
}

const duoMacChromeFeatures = `{"touch_supported":false,"platform_authenticator_status":"available","webauthn_supported":true,"screen_resolution_height":956,"screen_resolution_width":1470,"screen_color_depth":30,"is_uvpa_available":true,"client_capabilities_uvpa":true}`

func duoClientDetails(secCHUA string) (features, hints string, err error) {
	brands, fullVersions, err := clientHintBrands(secCHUA)
	if err != nil {
		return "", "", err
	}
	encoded, err := json.Marshal(duoClientHints{
		Brands:          brands,
		FullVersionList: fullVersions,
		Mobile:          false,
		Platform:        "macOS",
		PlatformVersion: "14.8.4",
		UAFullVersion:   chromeVersion,
	})
	if err != nil {
		return "", "", fmt.Errorf("encode Duo client hints: %w", err)
	}
	return duoMacChromeFeatures, base64.StdEncoding.EncodeToString(encoded), nil
}

func clientHintBrands(secCHUA string) (brands, fullVersions []duoBrand, err error) {
	matches := clientHintBrandPattern.FindAllStringSubmatch(secCHUA, -1)
	if len(matches) == 0 {
		return nil, nil, errors.New("mimic did not provide valid sec-ch-ua client hints")
	}
	majorVersion := strings.SplitN(chromeVersion, ".", 2)[0]
	for _, match := range matches {
		brand, version := match[1], match[2]
		brands = append(brands, duoBrand{Brand: brand, Version: version})
		fullVersion := version + ".0.0.0"
		if version == majorVersion && (brand == "Chromium" || brand == "Google Chrome") {
			fullVersion = chromeVersion
		}
		fullVersions = append(fullVersions, duoBrand{Brand: brand, Version: fullVersion})
	}
	return brands, fullVersions, nil
}

func jsonStrings(data []byte) map[string]string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return map[string]string{}
	}
	return collectJSONStrings(value)
}

func collectJSONStrings(value any) map[string]string {
	stringsByKey := make(map[string]string)
	var collect func(any)
	collect = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, nested := range typed {
				if stringValue, ok := nested.(string); ok && stringsByKey[key] == "" {
					stringsByKey[key] = stringValue
					var embedded any
					if json.Unmarshal([]byte(stringValue), &embedded) == nil {
						collect(embedded)
					}
				}
				collect(nested)
			}
		case []any:
			for _, nested := range typed {
				collect(nested)
			}
		}
	}
	collect(value)
	return stringsByKey
}

func validateDuoFactor(status int, body []byte) error {
	if status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden {
		return ErrBypassRejected
	}
	if status != http.StatusOK {
		return fmt.Errorf("duo passcode factor returned %d", status)
	}
	var payload struct {
		Stat     string `json:"stat"`
		Response struct {
			AuthnEvaluation struct {
				IsAllowed bool `json:"is_allowed"`
			} `json:"authn_evaluation"`
		} `json:"response"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return errors.New("duo passcode factor returned an invalid response")
	}
	if payload.Stat != "OK" || !payload.Response.AuthnEvaluation.IsAllowed {
		return ErrBypassRejected
	}
	return nil
}
