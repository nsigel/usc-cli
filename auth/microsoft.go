package auth

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"maps"
	"regexp"
	"sort"
	"strings"

	"github.com/nsigel/usc-cli/internal/dom"
	http "github.com/saucesteals/fhttp"
)

func (a *authenticator) continueMicrosoft(ctx context.Context, current *page) (*page, error) {

	if form, ok := microsoftRedirectForm(current); ok {
		return a.submit(ctx, current, form)
	}
	if current.URL.Path == "/login.srf" {

		bootstrap, err := a.get(ctx, "https://login.live.com/Me.htm", current.URL)
		if err != nil {
			return nil, err
		}
		if bootstrap.Status != http.StatusOK {
			return nil, fmt.Errorf("microsoft account bootstrap returned %d", bootstrap.Status)
		}
		a.rememberMicrosoft(bootstrap.Body)
	}
	if form, ok, err := microsoftForm(current, a.microsoft); err != nil {
		return nil, err
	} else if ok {
		return a.submit(ctx, current, form)
	}
	if form, ok := firstForm(current); ok {
		return a.submit(ctx, current, form)
	}
	if destination, ok := staticRedirect(current); ok {
		return a.get(ctx, destination.String(), current.URL)
	}
	return nil, noContinuation(current)
}

func (a *authenticator) rememberMicrosoft(body []byte) {
	for key, value := range microsoftValues(body) {
		if value != "" {
			a.microsoft[key] = value
		}
	}
}

func microsoftForm(page *page, remembered map[string]string) (form, bool, error) {
	thisPage := microsoftValues(page.Body)
	action := firstValue(thisPage, "urlPost", "urlPostMsa")
	destination, err := page.URL.Parse(action)
	if action == "" || err != nil || destination.Path != "/appverify" && destination.Path != "/kmsi" {
		return form{}, false, nil
	}
	state, err := microsoftState(page, thisPage, remembered)
	if err != nil {
		return form{}, false, err
	}
	fields := []field{{Name: "LoginOptions", Value: "1"}, {Name: "type", Value: "28"}}
	if destination.Path == "/appverify" {
		fields = []field{{Name: "ContinueAuth", Value: "true"}}
	}
	fields = append(fields, state.ctx, state.requestID, state.flowToken)
	if destination.Path == "/appverify" {
		fields = append(fields, field{Name: "iscsrfspeedbump", Value: "true"})
	}
	fields = append(fields, state.canary, state.pageTime)
	return form{Method: http.MethodPost, Action: action, Fields: fields}, true, nil
}

type microsoftSSOState struct {
	ctx, requestID, flowToken, canary, pageTime field
}

func microsoftState(page *page, thisPage, remembered map[string]string) (microsoftSSOState, error) {
	known := make(map[string]string, len(remembered))
	maps.Copy(known, remembered)
	for key, value := range thisPage {
		if value != "" {
			known[key] = value
		}
	}
	state := microsoftSSOState{
		ctx:       field{Name: "ctx", Value: firstValue(known, "sCtx", "ctx")},
		requestID: field{Name: "hpgrequestid", Value: firstValue(known, "sessionId", "hpgrequestid")},
		flowToken: field{Name: "flowToken", Value: firstValue(known, "sFT", "flowToken")},
		canary:    field{Name: "canary", Value: known["canary"]},

		pageTime: field{Name: "i19", Value: cmp.Or(known["i19"], "0")},
	}
	var missing []string
	for _, required := range []field{state.ctx, state.requestID, state.flowToken, state.canary} {
		if required.Value == "" {
			missing = append(missing, required.Name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return microsoftSSOState{}, fmt.Errorf("microsoft %s did not expose required SSO state: %s", page.URL.Path, strings.Join(missing, ","))
	}
	return state, nil
}

func microsoftRedirectForm(page *page) (form, bool) {
	config := microsoftConfig(page.Body)
	if config == nil {
		return form{}, false
	}
	action, _ := config["urlPost"].(string)
	parameters, _ := config["oPostParams"].(map[string]any)
	if action == "" || len(parameters) == 0 {
		return form{}, false
	}
	fields := make([]field, 0, len(parameters))
	for name, rawValue := range parameters {
		value, ok := rawValue.(string)
		if !ok {
			return form{}, false
		}

		fields = append(fields, field{
			Name:  html.UnescapeString(name),
			Value: html.UnescapeString(value),
		})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	return form{Method: http.MethodPost, Action: action, Fields: fields}, true
}

func microsoftValues(body []byte) map[string]string {
	values := make(map[string]string)
	script := microsoftScript(body)
	for _, config := range []map[string]any{microsoftConfig(script), microsoftServerData(script)} {
		for key, value := range collectJSONStrings(config) {
			values[key] = value
		}
	}
	for key, value := range microsoftAssignments(script) {
		values[key] = value
	}
	for _, key := range []string{"sCtx", "sFT", "i19", "ctx", "flowToken"} {
		if value := configString(script, key); value != "" {
			values[key] = value
		}
		if values[key] == "" {
			values[key] = scriptString(script, key)
		}
	}
	return values
}

func microsoftScript(body []byte) []byte {
	var script strings.Builder
	for _, node := range dom.FindAll(dom.Parse(body), dom.Tag("script")) {
		script.WriteString(dom.RawText(node))
		script.WriteByte('\n')
	}
	return []byte(script.String())
}

func microsoftConfig(body []byte) map[string]any {
	return configAssignment(body, "$Config")
}

func microsoftServerData(body []byte) map[string]any {
	return configAssignment(body, "ServerData")
}

func configAssignment(body []byte, marker string) map[string]any {

	var best map[string]any
	bestScore := -1
	for offset := 0; offset < len(body); {
		index := bytes.Index(body[offset:], []byte(marker))
		if index < 0 {
			break
		}
		index += offset
		limit := min(index+96, len(body))
		equals := bytes.IndexByte(body[index:limit], '=')
		if equals >= 0 {
			if candidate := jsonObject(body[index+equals+1:]); candidate != nil {
				score := len(candidate)
				for _, key := range []string{"urlPost", "sCtx", "sFT"} {
					if _, ok := candidate[key]; ok {
						score += 10_000
					}
				}
				if score > bestScore {
					best = candidate
					bestScore = score
				}
			}
		}
		offset = index + len(marker)
	}
	return best
}

func jsonObject(body []byte) map[string]any {
	start := bytes.IndexByte(body, '{')
	if start < 0 {
		return nil
	}
	depth := 0
	var quote byte
	escaped := false
	for index := start; index < len(body); index++ {
		character := body[index]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		switch character {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var object map[string]any
				if json.Unmarshal(body[start:index+1], &object) == nil {
					return object
				}
				return nil
			}
		}
	}
	return nil
}

func configString(body []byte, key string) string {
	pattern := regexp.MustCompile(`(?s)"` + regexp.QuoteMeta(key) + `"\s*:\s*"((?:\\.|[^"\\])*)"`)
	matches := pattern.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return ""
	}
	var value string
	quoted := append([]byte{'"'}, matches[len(matches)-1][1]...)
	quoted = append(quoted, '"')
	if json.Unmarshal(quoted, &value) != nil {
		return ""
	}
	return value
}

var microsoftAssignmentPattern = regexp.MustCompile(`(?s)\$Config\.([A-Za-z0-9_]+)\s*=\s*["']([^"']*)["']`)

func microsoftAssignments(body []byte) map[string]string {
	values := make(map[string]string)
	for _, match := range microsoftAssignmentPattern.FindAllSubmatch(body, -1) {
		values[string(match[1])] = string(match[2])
	}
	return values
}

func scriptString(body []byte, key string) string {
	pattern := regexp.MustCompile(`(?s)(?:["']?` + regexp.QuoteMeta(key) + `["']?\s*[:=]\s*["'])([^"']*)`)
	match := pattern.FindSubmatch(body)
	if len(match) == 2 {
		return string(match[1])
	}
	return ""
}

func firstValue(values map[string]string, names ...string) string {
	for _, name := range names {
		if values[name] != "" {
			return values[name]
		}
	}
	return ""
}

func (a *authenticator) microsoftStep(ctx context.Context, current *page, credentials Credentials) (*page, error) {
	if needsMicrosoftReload(current) {
		return a.reloadMicrosoft(ctx, current)
	}
	config := microsoftConfig(microsoftScript(current.Body))
	switch pageID, _ := config["pgid"].(string); pageID {
	case "ConvergedSignIn":
		return a.submitMicrosoftSignIn(ctx, current, config, credentials)
	case "ConvergedTFA":
		return a.beginExternalVerification(ctx, current, config, credentials)
	}
	return a.continueMicrosoft(ctx, current)
}

func (a *authenticator) submitMicrosoftSignIn(ctx context.Context, current *page, config map[string]any, credentials Credentials) (*page, error) {
	if credentials.Username == "" || credentials.Password == "" {
		return nil, ErrCredentialsRequired
	}
	if a.passwordSent {

		code, _ := config["sErrorCode"].(string)
		if code == "50126" || code == "50034" {
			return nil, ErrPasswordRejected
		}
		return nil, fmt.Errorf("microsoft sign-in failed (AADSTS%s)", code)
	}
	action, _ := config["urlPost"].(string)
	destination, err := current.URL.Parse(action)
	if action == "" || err != nil || !isMicrosoftHost(destination) || !strings.HasSuffix(destination.Path, "/login") {
		return nil, errors.New("microsoft sign-in page has no credential form")
	}
	state, err := microsoftState(current, configStrings(config), a.microsoft)
	if err != nil {
		return nil, err
	}
	a.passwordSent = true
	username := microsoftUsername(credentials.Username)

	return a.submit(ctx, current, form{Method: http.MethodPost, Action: destination.String(), Fields: []field{
		{Name: "i13", Value: "0"},
		{Name: "login", Value: username},
		{Name: "loginfmt", Value: username},
		{Name: "type", Value: "11"},
		{Name: "LoginOptions", Value: "3"},
		{Name: "lrt"}, {Name: "lrtPartition"}, {Name: "hisRegion"}, {Name: "hisScaleUnit"},
		{Name: "passwd", Value: credentials.Password},
		{Name: "ps", Value: "2"},
		{Name: "psRNGCDefaultType"}, {Name: "psRNGCEntropy"}, {Name: "psRNGCSLK"},
		state.canary, state.ctx, state.requestID, state.flowToken,
	}})
}

func microsoftUsername(username string) string {
	if strings.Contains(username, "@") {
		return username
	}
	return username + "@usc.edu"
}

func (a *authenticator) beginExternalVerification(ctx context.Context, current *page, config map[string]any, credentials Credentials) (*page, error) {

	if a.duoStarted {
		return nil, errors.New("microsoft asked for verification again after Duo")
	}
	proofs, _ := config["arrUserProofs"].([]any)
	var methodID string
	for _, proof := range proofs {
		if method, _ := proof.(map[string]any); method != nil && method["authMethodId"] == "ExternalAuth" {
			methodID, _ = method["externalAuthenticationMethodId"].(string)
		}
	}
	action, _ := config["urlExternalAuthRedirect"].(string)
	destination, err := current.URL.Parse(action)
	if methodID == "" || action == "" || err != nil || !isMicrosoftHost(destination) {
		return nil, errors.New("microsoft asked for a verification method other than USC Duo")
	}
	state, err := microsoftState(current, configStrings(config), a.microsoft)
	if err != nil {
		return nil, err
	}
	a.duoStarted = true
	return a.submit(ctx, current, form{Method: http.MethodPost, Action: destination.String(), Fields: []field{
		state.ctx, state.flowToken, {Name: "externalAuthenticationMethodId", Value: methodID}, state.canary,
	}})
}

func configStrings(config map[string]any) map[string]string {
	values := make(map[string]string, len(config))
	for key, value := range config {
		if text, ok := value.(string); ok {
			values[key] = text
		}
	}
	return values
}
