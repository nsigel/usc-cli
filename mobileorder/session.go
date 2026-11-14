package mobileorder

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/internal/dom"
	"github.com/nsigel/usc-cli/site"
)

const apiOrigin = "https://mobileorderprodapi.transactcampus.com"

// Open completes USC SSO and keeps the Transact identity in memory.
func Open(ctx context.Context, options auth.Options) (*Client, error) {
	s, err := auth.OpenSession(options, auth.CookiePolicy{})
	if err != nil {
		return nil, err
	}
	r, err := s.Authenticate(ctx, apiOrigin+"/api_user/samllogin?campusid=155", "")
	if r != nil {
		defer r.Body.Close()
	}
	if err != nil {
		return nil, auth.ForSite(site.MobileOrder, err)
	}
	if r.Request.URL.Scheme != "https" || r.Request.URL.Host != "mobileorderprodapi.transactcampus.com" || r.Request.URL.Path != "/api_user/samlsuccess" {
		return nil, fmt.Errorf("mobileorder: unexpected SSO callback")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		return nil, errors.New("mobileorder: read SSO callback")
	}
	doc := dom.Parse(data)
	if dom.Attr(dom.FindFirst(doc, dom.ID("from_reg")), "value") != "1" {
		return nil, errors.New("mobileorder: SSO callback is not a sign-in exchange")
	}
	token := dom.Attr(dom.FindFirst(doc, dom.ID("temp_token")), "value")
	if token == "" {
		return nil, auth.ForSite(site.MobileOrder, auth.ErrSessionInvalid)
	}
	c := New()
	c.http.CheckRedirect = func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }
	c.sessionID = strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(exchangeKey))
	mac.Write([]byte(token))
	var result struct {
		User userWire `json:"user"`
	}
	err = c.call(ctx, "registerwithcampusssotoken", map[string]any{"hash": hex.EncodeToString(mac.Sum(nil)), "temp_token": token, "language": "EN", "os_type": "1", "app_bundle_name": "com.transact.mobileorder"}, &result)
	if err != nil {
		return nil, err
	}
	if result.User.ID <= 0 || result.User.CampusID != 155 {
		return nil, errors.New("mobileorder: invalid session identity")
	}
	if result.User.Token == "" || strings.ContainsAny(result.User.Token, "\r\n") {
		return nil, errors.New("mobileorder: missing or invalid session token")
	}
	c.token = result.User.Token
	c.userID = result.User.ID
	if _, err = c.Account(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) call(ctx context.Context, endpoint string, input map[string]any, out any) error {
	if input == nil {
		input = map[string]any{}
	}
	input["userid"] = strconv.Itoa(c.userID)
	input["campusid"] = "155"
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiOrigin+"/api_user/"+endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("sessionid", c.sessionID)
	if c.token != "" {
		req.Header.Set("login_token", c.token)
		req.Header.Set("userid", strconv.Itoa(c.userID))
		req.Header.Set("campusid", "155")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return errors.New("mobileorder: API request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return auth.ForSite(site.MobileOrder, auth.ErrSessionInvalid)
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("mobileorder: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return errors.New("mobileorder: read API response")
	}
	var envelope struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return errors.New("mobileorder: invalid API response")
	}
	if envelope.Status != "OK" {
		if envelope.Reason == "TOKEN" {
			return auth.ForSite(site.MobileOrder, auth.ErrSessionInvalid)
		}
		if envelope.Reason != "" && len(envelope.Reason) < 64 && strings.IndexFunc(envelope.Reason, func(r rune) bool { return !(r >= 'A' && r <= 'Z' || r == '_') }) == -1 {
			return fmt.Errorf("mobileorder: API rejected request (%s)", envelope.Reason)
		}
		return errors.New("mobileorder: API rejected request")
	}
	if json.Unmarshal(data, out) != nil {
		return errors.New("mobileorder: invalid response data")
	}
	return nil
}
