package mobileorder

import (
	"context"
	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/site"
)

type userWire struct {
	ID       int    `json:"userid"`
	CampusID int    `json:"campusid"`
	Token    string `json:"login_token"`
}

// Account identifies the signed-in user without exposing authentication tokens.
type Account struct {
	ID       int `json:"id"`
	CampusID int `json:"campus_id"`
}

// Account verifies the current token and returns the account identity.
func (c *Client) Account(ctx context.Context) (*Account, error) {
	if c.token == "" {
		return nil, auth.ForSite(site.MobileOrder, auth.ErrCredentialsRequired)
	}
	var result struct {
		User userWire `json:"user"`
	}
	if err := c.call(ctx, "getuser", nil, &result); err != nil {
		return nil, err
	}
	if result.User.ID != c.userID || result.User.CampusID != 155 {
		return nil, auth.ForSite(site.MobileOrder, auth.ErrSessionInvalid)
	}
	return &Account{ID: result.User.ID, CampusID: result.User.CampusID}, nil
}
