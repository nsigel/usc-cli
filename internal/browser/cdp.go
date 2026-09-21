package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

// ErrCDPUnavailable means the Chrome DevTools endpoint could not be reached.
var ErrCDPUnavailable = errors.New("chrome DevTools unavailable")

// Client is a minimal Chrome DevTools Protocol client for Storage.setCookies.
type Client struct {
	ws       *websocket.Conn
	endpoint string
	nextID   atomic.Int64
}

type versionResponse struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpRequest struct {
	ID     int64          `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id"`
	Error  *cdpError       `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

type cdpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Connect dials a CDP target (HTTP endpoint or websocket debugger URL).
func Connect(ctx context.Context, target string) (*Client, error) {
	resolved, err := ResolveTarget(target)
	if err != nil {
		return nil, err
	}

	wsURL := resolved
	display := resolved
	if strings.HasPrefix(strings.ToLower(resolved), "http://") || strings.HasPrefix(strings.ToLower(resolved), "https://") {
		var err error
		wsURL, err = fetchWebSocketURL(ctx, resolved)
		if err != nil {
			return nil, err
		}
		display = resolved
	}

	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: connect to %s: %v", ErrCDPUnavailable, display, err)
	}
	conn.SetReadLimit(16 << 20)

	return &Client{ws: conn, endpoint: display}, nil
}

// Endpoint returns the resolved CDP HTTP endpoint or websocket URL used to connect.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// Close closes the websocket connection.
func (c *Client) Close() error {
	if c == nil || c.ws == nil {
		return nil
	}
	return c.ws.Close(websocket.StatusNormalClosure, "")
}

// SetCookies injects cookies via Storage.setCookies, falling back to one-by-one
// when the batch call fails. Returns how many cookies were set successfully.
// Cookie values are never logged.
func (c *Client) SetCookies(ctx context.Context, cookies []CookieParam) (int, error) {
	if len(cookies) == 0 {
		return 0, nil
	}
	if err := c.call(ctx, "Storage.setCookies", map[string]any{"cookies": cookies}); err == nil {
		return len(cookies), nil
	}

	set := 0
	var firstErr error
	for _, cookie := range cookies {
		name := cookie.Name
		err := c.call(ctx, "Storage.setCookies", map[string]any{
			"cookies": []CookieParam{cookie},
		})
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("set cookie %q: %w", name, err)
			}
			continue
		}
		set++
	}
	if set == 0 && firstErr != nil {
		return 0, firstErr
	}
	return set, nil
}

func (c *Client) call(ctx context.Context, method string, params map[string]any) error {
	id := c.nextID.Add(1)
	payload, err := json.Marshal(cdpRequest{ID: id, Method: method, Params: params})
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := c.ws.Write(writeCtx, websocket.MessageText, payload); err != nil {
		return fmt.Errorf("CDP write %s: %w", method, err)
	}

	readCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		readCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	for {
		_, data, err := c.ws.Read(readCtx)
		if err != nil {
			return fmt.Errorf("CDP read %s: %w", method, err)
		}
		var resp cdpResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		if resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return fmt.Errorf("CDP %s: %s", method, resp.Error.Message)
		}
		return nil
	}
}

func fetchWebSocketURL(ctx context.Context, httpEndpoint string) (string, error) {
	endpoint := strings.TrimRight(httpEndpoint, "/") + "/json/version"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: reach %s: %v", ErrCDPUnavailable, httpEndpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("%w: %s returned %s: %s", ErrCDPUnavailable, endpoint, resp.Status, strings.TrimSpace(string(body)))
	}
	var version versionResponse
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		return "", fmt.Errorf("%w: decode version from %s: %v", ErrCDPUnavailable, endpoint, err)
	}
	if version.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("%w: %s did not return webSocketDebuggerUrl", ErrCDPUnavailable, endpoint)
	}
	return version.WebSocketDebuggerURL, nil
}
