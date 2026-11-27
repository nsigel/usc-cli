package browser

import (
	"context"

	"github.com/nsigel/usc-cli/auth"
)

type SyncResult struct {
	CDP       string `json:"cdp"`
	Attempted int    `json:"attempted"`
	Set       int    `json:"set"`
	Skipped   int    `json:"skipped"`
}

func SyncCookies(ctx context.Context, sessionFile, target string) (*SyncResult, error) {
	stored, err := auth.LoadStoredCookies(sessionFile)
	if err != nil {
		return nil, err
	}

	converted := CookiesToParams(stored)
	client, err := Connect(ctx, target)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	set, err := client.SetCookies(ctx, converted.Cookies)
	if err != nil {
		return nil, err
	}

	return &SyncResult{
		CDP:       client.Endpoint(),
		Attempted: len(converted.Cookies),
		Set:       set,
		Skipped:   converted.Skipped,
	}, nil
}
