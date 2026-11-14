// Package mobileorder provides Transact Mobile Ordering discovery and authenticated reads.
package mobileorder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client accesses Transact Mobile Ordering. Its authentication token stays private.
type Client struct {
	http      *http.Client
	token     string
	userID    int
	sessionID string
}

// New constructs a client without credentials or persistent state.
func New() *Client { return &Client{http: &http.Client{Timeout: 30 * time.Second}} }

func (c *Client) campuses(ctx context.Context) ([]Campus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://mobileorderprodapi.transactcampus.com/api_user/getcampuseswithbundlename", bytes.NewBufferString(`{"app_bundle_name":"com.transact.mobileorder"}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mobileorder: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mobileorder: HTTP %d", resp.StatusCode)
	}
	var result struct {
		Status   string       `json:"status"`
		Campuses []campusWire `json:"campuses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mobileorder: decode directory: %w", err)
	}
	if result.Status != "OK" {
		return nil, fmt.Errorf("mobileorder: campus discovery failed")
	}
	campuses := make([]Campus, 0, len(result.Campuses))
	for _, x := range result.Campuses {
		campuses = append(campuses, Campus{ID: x.ID, Name: x.Name, City: x.City, Timezone: x.Timezone})
	}
	return campuses, nil
}
