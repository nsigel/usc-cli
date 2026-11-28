package auth

import (
	"context"
	"encoding/json"
	"errors"
	http "github.com/saucesteals/fhttp"
	"time"
)

type DuoMethod struct {
	Type     string `json:"type"`
	DeviceID string `json:"device_id,omitempty"`
	Name     string `json:"name,omitempty"`
}
type DuoMethodsRequired struct{ Methods []DuoMethod }

func (e *DuoMethodsRequired) Error() string {
	return "Choose an available Duo method to authorize device creation"
}
func (e *DuoMethodsRequired) ErrorCode() string { return "duo_method_required" }
func (e *DuoMethodsRequired) Unwrap() error     { return ErrCredentialsRequired }

func (a *authenticator) duoPush(ctx context.Context, p duoPrompt) (string, error) {
	allowed := false
	for _, f := range a.duoFactors {
		if f.Type == "push" && f.DeviceID == a.duoPushDevice {
			allowed = true
		}
	}
	if !allowed {
		return "", &DuoMethodsRequired{Methods: a.duoFactors}
	}
	pushCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"authkey": p.authKey, "pkey": a.duoPushDevice})
	r, err := a.do(pushCtx, navigation{method: http.MethodPost, url: p.base + "/auth/factors/push/auth", body: body, contentType: "application/json", referer: p.page.URL, duoTraceGroup: p.traceGroup})
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if pushCtx.Err() != nil {
			return "", ErrDuoPushTimedOut
		}
		return "", errors.New("Duo push request failed")
	}
	var started struct {
		Stat     string `json:"stat"`
		Response struct {
			TxID string `json:"push_txid"`
			Code string `json:"step_up_code"`
		} `json:"response"`
	}
	if r.Status != 200 || json.Unmarshal(r.Body, &started) != nil || started.Stat != "OK" || started.Response.TxID == "" {
		return "", errors.New("Duo did not start the push")
	}
	if a.duoPushStarted != nil {
		a.duoPushStarted(started.Response.Code)
	}
	for {
		r, err = a.do(pushCtx, navigation{url: p.base + "/auth/factors/push/status?" + encodeFields([]field{{Name: "authkey", Value: p.authKey}, {Name: "push_txid", Value: started.Response.TxID}, {Name: "saw_good_news", Value: "false"}}), referer: p.page.URL, duoTraceGroup: p.traceGroup})
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			if pushCtx.Err() != nil {
				return "", ErrDuoPushTimedOut
			}
			return "", errors.New("Duo push status request failed")
		}
		var status struct {
			Stat     string `json:"stat"`
			Response struct {
				Result struct {
					Result     string `json:"result"`
					AuthResult struct {
						Evaluation struct {
							Allowed bool `json:"is_allowed"`
						} `json:"authn_evaluation"`
					} `json:"auth_result"`
				} `json:"result"`
			} `json:"response"`
		}
		if r.Status != 200 || json.Unmarshal(r.Body, &status) != nil || status.Stat != "OK" {
			return "", errors.New("invalid Duo push status")
		}
		result := status.Response.Result
		if result.AuthResult.Evaluation.Allowed {
			return jsonStrings(r.Body)["oidc_code"], nil
		}
		if result.Result == "FAILURE" || result.Result == "DENY" || result.Result == "denied" {
			return "", ErrDuoPushRejected
		}
		select {
		case <-pushCtx.Done():
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", ErrDuoPushTimedOut
		case <-time.After(time.Second):
		}
	}
}
