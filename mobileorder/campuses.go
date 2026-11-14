package mobileorder

import (
	"context"
	"fmt"
)

// Campus describes a campus in the Transact directory.
type Campus struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Timezone string `json:"timezone"`
}

type campusWire struct {
	ID       int    `json:"campusid"`
	Name     string `json:"name"`
	City     string `json:"city"`
	Timezone string `json:"timezone"`
}

// Campuses returns the public campus directory.
func (c *Client) Campuses(ctx context.Context) ([]Campus, error) { return c.campuses(ctx) }

// USC returns USC Campus Dining's directory entry.
func (c *Client) USC(ctx context.Context) (*Campus, error) {
	campuses, err := c.Campuses(ctx)
	if err != nil {
		return nil, err
	}
	for _, campus := range campuses {
		if campus.ID == 155 {
			return &campus, nil
		}
	}
	return nil, fmt.Errorf("mobileorder: USC Campus Dining missing from directory")
}
