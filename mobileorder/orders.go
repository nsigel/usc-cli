package mobileorder

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
)

// Order includes fulfillment status and receipt data without the account envelope.
type Order struct {
	ID         int             `json:"id"`
	LocationID int             `json:"location_id"`
	Barcode    string          `json:"barcode"`
	Total      int             `json:"total"`
	Placed     string          `json:"placed"`
	Complete   bool            `json:"complete"`
	Refunded   bool            `json:"refunded"`
	Items      json.RawMessage `json:"items"`
}
type orderWire struct {
	ID         int             `json:"orderid"`
	LocationID int             `json:"locationid"`
	Barcode    string          `json:"barcode_token"`
	Total      int             `json:"grand_total"`
	Placed     string          `json:"order_datetime"`
	Complete   int             `json:"iscomplete"`
	Refunded   int             `json:"isrefunded"`
	Items      json.RawMessage `json:"items"`
}

func (c *Client) Orders(ctx context.Context, pending bool) ([]Order, error) {
	endpoint := "getpastorders"
	if pending {
		endpoint = "getpendingorders"
	}
	var r struct {
		Orders  []orderWire `json:"orders"`
		Pending []orderWire `json:"pending_orders"`
	}
	if err := c.call(ctx, endpoint, nil, &r); err != nil {
		return nil, err
	}
	rows := r.Orders
	if pending {
		rows = r.Pending
	}
	out := make([]Order, 0, len(rows))
	for _, x := range rows {
		out = append(out, Order{x.ID, x.LocationID, x.Barcode, x.Total, x.Placed, x.Complete == 1, x.Refunded == 1, x.Items})
	}
	return out, nil
}

// OrderProgress is a staged checkout result. Only stage "success" means completed checkout.
type OrderProgress struct {
	ID      int    `json:"id"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}
type progressWire struct {
	ID      int    `json:"order_status_orderid"`
	Stage   string `json:"order_status_stage"`
	Message string `json:"order_status_message"`
}

// ContinueOrder advances/checks an already staged purchase; it may process payment.
func (c *Client) ContinueOrder(ctx context.Context, id int) (*OrderProgress, error) {
	if id <= 0 {
		return nil, errors.New("mobileorder: order ID must be positive")
	}
	var r progressWire
	if err := c.call(ctx, "processorderstatuscheck", map[string]any{"orderid": strconv.Itoa(id)}, &r); err != nil {
		return nil, &OutcomeUnknown{ID: id}
	}
	if r.Stage == "" {
		return nil, &OutcomeUnknown{ID: id}
	}
	return &OrderProgress{r.ID, r.Stage, r.Message}, nil
}

// Acknowledgement confirms an action accepted by the API.
type Acknowledgement struct {
	Accepted bool `json:"accepted"`
}

func (c *Client) CancelOrder(ctx context.Context, id int) (*Acknowledgement, error) {
	if id <= 0 {
		return nil, errors.New("mobileorder: order ID must be positive")
	}
	var r struct{}
	if err := c.call(ctx, "cancelpendingorder", map[string]any{"orderid": strconv.Itoa(id)}, &r); err != nil {
		return nil, err
	}
	return &Acknowledgement{true}, nil
}

// OutcomeUnknown means a purchase may have been accepted. Never automatically resubmit.
type OutcomeUnknown struct{ ID int }

func (e *OutcomeUnknown) Error() string {
	return "mobileorder: order outcome unknown; inspect pending and past orders before submitting again"
}
func (e *OutcomeUnknown) ErrorCode() string { return "mobileorder_order_unknown" }

// CheckInOrder announces arrival for a selected order using its check-in barcode.
func (c *Client) CheckInOrder(ctx context.Context, id int, barcode string) (*Acknowledgement, error) {
	if id <= 0 || barcode == "" {
		return nil, errors.New("mobileorder: order ID and check-in barcode are required")
	}
	var r struct{}
	if err := c.call(ctx, "checkinorder", map[string]any{"orderid": strconv.Itoa(id), "checkin_barcode": barcode}, &r); err != nil {
		return nil, err
	}
	return &Acknowledgement{true}, nil
}
