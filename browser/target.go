package browser

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const defaultCDPTarget = "127.0.0.1:9222"

// ResolveTarget normalizes a CDP target string into an HTTP endpoint or a
// websocket debugger URL. Empty target defaults to 127.0.0.1:9222.
//
// Accepted forms:
//   - port: "9224"
//   - host:port: "127.0.0.1:9224"
//   - http(s) CDP endpoint: "http://127.0.0.1:9224"
//   - websocket debugger URL: "ws://127.0.0.1:9224/devtools/browser/..."
func ResolveTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		target = defaultCDPTarget
	}

	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
		parsed, err := url.Parse(target)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("invalid CDP websocket URL %q", target)
		}
		return target, nil
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		parsed, err := url.Parse(target)
		if err != nil || parsed.Host == "" {
			return "", fmt.Errorf("invalid CDP HTTP URL %q", target)
		}
		parsed.Path = strings.TrimRight(parsed.Path, "/")
		if parsed.Path == "" {
			parsed.RawQuery = ""
			parsed.Fragment = ""
			return parsed.String(), nil
		}
		// Allow paths; callers append /json/version when needed.
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return strings.TrimRight(parsed.String(), "/"), nil
	}

	if port, err := strconv.Atoi(target); err == nil {
		if port <= 0 || port > 65535 {
			return "", fmt.Errorf("invalid CDP port %q", target)
		}
		return fmt.Sprintf("http://127.0.0.1:%d", port), nil
	}

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		return "", fmt.Errorf("invalid CDP target %q: want port, host:port, http(s) URL, or ws(s) URL", target)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "", fmt.Errorf("invalid CDP port in %q", target)
	}
	return fmt.Sprintf("http://%s", net.JoinHostPort(host, port)), nil
}
