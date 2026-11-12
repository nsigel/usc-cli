package classes

import (
	"context"
	"strconv"
)

// Term is a term the Schedule of Classes currently lists.
type Term struct {
	// Code is the five-digit term code, the year followed by 1 (Spring),
	// 2 (Summer), or 3 (Fall): "20271".
	Code   string `json:"code"`
	Season string `json:"season"`
	Year   int    `json:"year"`
}

type termResponse struct {
	TermCode int    `json:"termCode"`
	Season   string `json:"season"`
	Year     int    `json:"year"`
}

// Terms lists the terms the Schedule of Classes currently publishes, newest
// first.
func (c *Client) Terms(ctx context.Context) ([]Term, error) {
	var wire []termResponse
	if err := c.get(ctx, "Terms/Active", nil, &wire); err != nil {
		return nil, err
	}
	terms := make([]Term, len(wire))
	for index, wireTerm := range wire {
		terms[index] = Term{
			Code:   strconv.Itoa(wireTerm.TermCode),
			Season: wireTerm.Season,
			Year:   wireTerm.Year,
		}
	}
	return terms, nil
}
