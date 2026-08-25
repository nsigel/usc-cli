package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	http "github.com/saucesteals/fhttp"
)

func (a *authenticator) duo(ctx context.Context, prompt *page, bypassCode string) (*page, error) {
	authKey := prompt.URL.Query().Get("authkey")
	traceGroup := prompt.URL.Query().Get("req_trace_group")
	if authKey == "" || traceGroup == "" {
		return nil, errors.New("duo prompt is missing authkey or req_trace_group")
	}

	baseURL := "https://" + prompt.URL.Host + prompt.URL.Path
	features, hints := duoClientDetails()
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

func duoClientDetails() (string, string) {
	features := `{"touch_supported":false,"platform_authenticator_status":"available","webauthn_supported":true,"screen_resolution_height":956,"screen_resolution_width":1470,"screen_color_depth":30,"is_uvpa_available":true,"client_capabilities_uvpa":true}`
	hints := `{"brands":[{"brand":"Chromium","version":"151"},{"brand":"Not=A?Brand","version":"99"}],"fullVersionList":[{"brand":"Chromium","version":"151.0.7922.76"},{"brand":"Not=A?Brand","version":"99.0.0.0"}],"mobile":false,"platform":"macOS","platformVersion":"14.8.4","uaFullVersion":"151.0.7922.76"}`
	return features, base64.StdEncoding.EncodeToString([]byte(hints))
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
