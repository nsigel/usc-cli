package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	stdhtml "html"
	"regexp"
	"sort"
	"strings"

	http "github.com/saucesteals/fhttp"
	"golang.org/x/net/html"
)

func (a *authenticator) continueMicrosoft(ctx context.Context, current *page) (*page, error) {
	if form, ok := microsoftRedirectForm(current); ok {
		return a.submit(ctx, current, form)
	}
	if current.URL.Path == "/login.srf" {
		bootstrap, err := a.do(ctx, http.MethodGet, "https://login.live.com/Me.htm", nil, "", current.URL, "")
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
	if form, ok := autoForm(current); ok {
		return a.submit(ctx, current, form)
	}
	if destination, ok := htmlNavigation(current); ok {
		return a.do(ctx, http.MethodGet, destination.String(), nil, "", current.URL, "")
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

func microsoftForm(page *page, stored map[string]string) (form, bool, error) {
	values := make(map[string]string, len(stored))
	for key, value := range stored {
		values[key] = value
	}
	current := microsoftValues(page.Body)
	for key, value := range current {
		if value != "" {
			values[key] = value
		}
	}

	// urlPost belongs to this page. Reusing it from an earlier page can turn a
	// terminal SAML auto-post into a duplicate Microsoft postback.
	action := firstValue(current, "urlPost", "urlPostMsa")
	if action == "" {
		return form{}, false, nil
	}
	destination, err := page.URL.Parse(action)
	if err != nil {
		return form{}, false, nil
	}
	if destination.Path != "/appverify" && destination.Path != "/kmsi" {
		return form{}, false, nil
	}

	ctxValue := firstValue(values, "sCtx", "ctx")
	requestID := firstValue(values, "sessionId", "hpgrequestid")
	flowToken := firstValue(values, "sFT", "flowToken")
	canary := values["canary"]
	required := map[string]string{
		"ctx":          ctxValue,
		"flowToken":    flowToken,
		"canary":       canary,
		"hpgrequestid": requestID,
	}
	missing := make([]string, 0, len(required))
	for name, value := range required {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return form{}, false, fmt.Errorf("microsoft %s did not expose required SSO state: %s", page.URL.Path, strings.Join(missing, ","))
	}

	pageTime := values["i19"]
	if pageTime == "" {
		// Microsoft uses i19 for client-side timing telemetry, not SSO state.
		pageTime = "0"
	}
	common := []field{
		{Name: "ctx", Value: ctxValue},
		{Name: "hpgrequestid", Value: requestID},
		{Name: "flowToken", Value: flowToken},
		{Name: "canary", Value: canary},
		{Name: "i19", Value: pageTime},
	}
	if destination.Path == "/appverify" {
		return form{Method: http.MethodPost, Action: action, Fields: []field{
			{Name: "ContinueAuth", Value: "true"},
			common[0], common[1], common[2],
			{Name: "iscsrfspeedbump", Value: "true"},
			common[3], common[4],
		}}, true, nil
	}
	return form{Method: http.MethodPost, Action: action, Fields: []field{
		{Name: "LoginOptions", Value: "1"},
		{Name: "type", Value: "28"},
		common[0], common[1], common[2], common[3], common[4],
	}}, true, nil
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
		// BssoInterrupt_Core HTML-unescapes names and values before creating
		// hidden inputs. Sending the encoded config changes signed SAML data.
		fields = append(fields, field{
			Name:  stdhtml.UnescapeString(name),
			Value: stdhtml.UnescapeString(value),
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

// microsoftScript limits JavaScript extraction to script elements. Microsoft
// configuration strings can also appear in page markup or error messages, and
// those must not be treated as executable SSO state.
func microsoftScript(body []byte) []byte {
	document := parseHTML(body)
	if document == nil {
		return nil
	}
	var script strings.Builder
	walkHTML(document, func(node *html.Node) bool {
		if node.Type == html.ElementNode && node.Data == "script" {
			script.WriteString(htmlText(node))
			script.WriteByte('\n')
		}
		return false
	})
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
