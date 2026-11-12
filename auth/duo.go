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

// duoPrompt is the state Duo's prompt page hands to its browser client.
type duoPrompt struct {
	page       *page
	base       string
	authKey    string
	traceGroup string // correlates the requests below; protocol state, not telemetry
}

// duo completes Duo's prompt with a bypass code and returns the page Duo
// sends the browser back to.
func (a *authenticator) duo(ctx context.Context, current *page, bypassCode string) (*page, error) {
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
	oidcCode, err := a.duoSubmitBypass(ctx, prompt, bypassCode)
	if err != nil {
		return nil, err
	}
	return a.duoFinalize(ctx, prompt, oidcCode)
}

// duoPreflight makes the capability and policy requests Duo's browser client
// makes. Duo only accepts a bypass code after both.
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
	return nil
}

// duoSubmitBypass is the only place the bypass code is sent. It returns the
// OIDC code Duo issues for some integrations, or "".
func (a *authenticator) duoSubmitBypass(ctx context.Context, prompt duoPrompt, bypassCode string) (string, error) {
	body, err := json.Marshal(map[string]string{"authkey": prompt.authKey, "bypass_code": bypassCode})
	if err != nil {
		return "", err
	}
	factor, err := a.do(ctx, navigation{
		method: http.MethodPost, url: prompt.base + "/auth/factors/bypass_code",
		body: body, contentType: "application/json",
		referer: prompt.page.URL, duoTraceGroup: prompt.traceGroup,
	})
	if err != nil {
		return "", err
	}
	if err := validateDuoFactor(factor.Status, factor.Body); err != nil {
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

// duoMacChromeFeatures is what Duo's prompt reports for Chrome on a MacBook.
const duoMacChromeFeatures = `{"touch_supported":false,"platform_authenticator_status":"available","webauthn_supported":true,"screen_resolution_height":956,"screen_resolution_width":1470,"screen_color_depth":30,"is_uvpa_available":true,"client_capabilities_uvpa":true}`

// duoClientDetails describes the browser to Duo, consistently with the
// sec-ch-ua header the transport sends.
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

// clientHintBrands parses a sec-ch-ua header into its brands and the full
// versions a browser would report for them.
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
