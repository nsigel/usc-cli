package mobileorder

import (
	"context"
	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
)

// Location is a dining location available to the signed-in account.
type Location struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	PickupStatus string `json:"pickup_status"`
}

// Locations returns the current campus dining locations.
func (c *Client) Locations(ctx context.Context) ([]Location, error) {
	if c.token == "" {
		return nil, auth.ForSite(site.MobileOrder, auth.ErrCredentialsRequired)
	}
	var result struct {
		Locations []struct {
			ID           int    `json:"locationid"`
			Name         string `json:"name"`
			PickupStatus string `json:"app_takeout_status_display_text"`
		} `json:"locations"`
	}
	if err := c.call(ctx, "getcampuslocations", nil, &result); err != nil {
		return nil, err
	}
	locations := make([]Location, 0, len(result.Locations))
	for _, x := range result.Locations {
		locations = append(locations, Location{ID: x.ID, Name: x.Name, PickupStatus: x.PickupStatus})
	}
	return locations, nil
}
