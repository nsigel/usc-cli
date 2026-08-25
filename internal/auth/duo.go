package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	http "github.com/saucesteals/fhttp"
)

func (a *authenticator) duo(ctx context.Context, prompt *page, bypassCode string) (*page, error) {
	authKey := prompt.URL.Query().Get("authkey")
	traceGroup := prompt.URL.Query().Get("req_trace_group")
	if authKey == "" || traceGroup == "" {
		return nil, errors.New("duo prompt is missing authkey or req_trace_group")
	}

	// Duo exposes the bypass factor only after the same capability and policy
	// preflight its browser client performs. Skipping either request is rejected.
	baseURL := "https://" + prompt.URL.Host + prompt.URL.Path
	features, hints, err := duoClientDetails(a.duoClientHintUA)
	if err != nil {
		return nil, err
	}
	payloadURL := baseURL + "/auth/payload?" + encodeFields([]field{
		{Name: "authkey", Value: authKey},
		{Name: "browser_features", Value: features},
		{Name: "is_ipad", Value: "false"},
		{Name: "client_hints", Value: hints},
	})
	payload, err := a.do(ctx, http.MethodGet, payloadURL, nil, "", prompt.URL, "")
	if err != nil {
		return nil, err
	}
	if payload.Status != http.StatusOK {
		return nil, fmt.Errorf("duo auth/payload returned %d", payload.Status)
	}

	evaluationURL := baseURL + "/pre_authn/evaluation?" + encodeFields([]field{
		{Name: "authkey", Value: authKey},
		{Name: "browser_features", Value: features},
		{Name: "local_trust_choice", Value: "trusted"},
	})
	// Duo correlates policy evaluation, factor use, and finalization with this
	// trace group; it is protocol state rather than observability metadata.
	evaluation, err := a.do(ctx, http.MethodGet, evaluationURL, nil, "", prompt.URL, traceGroup)
	if err != nil {
		return nil, err
	}
	if evaluation.Status != http.StatusOK {
		return nil, fmt.Errorf("duo pre_authn/evaluation returned %d", evaluation.Status)
	}

	body, err := json.Marshal(map[string]string{"authkey": authKey, "bypass_code": bypassCode})
	if err != nil {
		return nil, err
	}
	factor, err := a.do(ctx, http.MethodPost, baseURL+"/auth/factors/bypass_code", body, "application/json", prompt.URL, traceGroup)
	if err != nil {
		return nil, err
	}
	if err := validateDuoFactor(factor.Status, factor.Body); err != nil {
		return nil, err
	}

	parameters := []field{{Name: "authkey", Value: authKey}}
	if oidcCode := jsonStrings(factor.Body)["oidc_code"]; oidcCode != "" {
		parameters = append(parameters, field{Name: "oidc_code", Value: oidcCode})
	}
	final, err := a.do(ctx, http.MethodGet, baseURL+"/auth/finalize_auth?"+encodeFields(parameters), nil, "", prompt.URL, traceGroup)
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
	return a.do(ctx, http.MethodGet, exitURL, nil, "", prompt.URL, "")
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

func duoClientDetails(clientHintUA string) (string, string, error) {
	// Mimic owns the UA-brand ordering and greased brand. Reusing its actual
	// sec-ch-ua value keeps this protocol field aligned with the HTTP/TLS
	// transport instead of maintaining a second, brittle implementation.
	matches := clientHintBrandPattern.FindAllStringSubmatch(clientHintUA, -1)
	if len(matches) == 0 {
		return "", "", errors.New("mimic did not provide valid sec-ch-ua client hints")
	}

	majorVersion := strings.SplitN(chromeVersion, ".", 2)[0]
	brands := make([]duoBrand, 0, len(matches))
	fullVersionList := make([]duoBrand, 0, len(matches))
	for _, match := range matches {
		brand, version := match[1], match[2]
		brands = append(brands, duoBrand{Brand: brand, Version: version})

		fullVersion := version + ".0.0.0"
		if version == majorVersion && (brand == "Chromium" || brand == "Google Chrome") {
			fullVersion = chromeVersion
		}
		fullVersionList = append(fullVersionList, duoBrand{Brand: brand, Version: fullVersion})
	}

	// These runtime capabilities are not included in Mimic's transport API.
	// They describe the supported macOS Chrome client used by that transport.
	features := `{"touch_supported":false,"platform_authenticator_status":"available","webauthn_supported":true,"screen_resolution_height":956,"screen_resolution_width":1470,"screen_color_depth":30,"is_uvpa_available":true,"client_capabilities_uvpa":true}`
	hints, err := json.Marshal(duoClientHints{
		Brands:          brands,
		FullVersionList: fullVersionList,
		Mobile:          false,
		Platform:        "macOS",
		PlatformVersion: "14.8.4",
		UAFullVersion:   chromeVersion,
	})
	if err != nil {
		return "", "", fmt.Errorf("encode Duo client hints: %w", err)
	}
	return features, base64.StdEncoding.EncodeToString(hints), nil
}

func jsonStrings(data []byte) map[string]string {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return map[string]string{}
	}
	return collectJSONStrings(value)
}

// Duo sometimes nests continuation values inside JSON-encoded strings, so a
// shallow unmarshal can silently miss the final URL or OIDC code.
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
		return fmt.Errorf("duo bypass factor returned %d", status)
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
		return errors.New("duo bypass factor returned an invalid response")
	}
	if payload.Stat != "OK" || !payload.Response.AuthnEvaluation.IsAllowed {
		return ErrBypassRejected
	}
	return nil
}
