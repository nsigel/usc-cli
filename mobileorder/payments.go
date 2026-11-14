package mobileorder

import "context"

// PaymentMethod identifies an existing account payment method; it never contains a token.
type PaymentMethod struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func (c *Client) PaymentMethods(ctx context.Context) ([]PaymentMethod, error) {
	var r struct {
		User struct {
			Meal  int `json:"has_mealplancard"`
			Card1 int `json:"has_creditcard_1"`
			Card2 int `json:"has_creditcard_2"`
		} `json:"user"`
	}
	if err := c.call(ctx, "getuser", nil, &r); err != nil {
		return nil, err
	}
	out := make([]PaymentMethod, 0, 3)
	if r.User.Meal == 1 {
		out = append(out, PaymentMethod{ID: 0, Name: "Campus card"})
	}
	if r.User.Card1 == 1 {
		out = append(out, PaymentMethod{ID: 1, Name: "Saved card 1"})
	}
	if r.User.Card2 == 1 {
		out = append(out, PaymentMethod{ID: 2, Name: "Saved card 2"})
	}
	return out, nil
}
