package mobileorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

// Cart is an explicit purchase selection. Amounts are integer cents.
// Repeat an item entry to request multiple units, as the Android app does.
type Cart struct {
	LocationID        int         `json:"location_id"`
	Fulfillment       Fulfillment `json:"fulfillment"`
	PaymentMethod     *int        `json:"payment_method"`
	TenderID          int         `json:"tender_id"`
	SecondaryTenderID int         `json:"secondary_tender_id"`
	Date              *string     `json:"date"`
	Time              *string     `json:"time"`
	PickupSpotID      int         `json:"pickup_spot_id"`
	Comment           string      `json:"comment"`
	CheckoutChoices   []int       `json:"checkout_choices"`
	Items             []CartItem  `json:"items"`
}
type CartItem struct {
	ItemID       int          `json:"itemid"`
	SectionID    int          `json:"sectionid"`
	MealExchange bool         `json:"meal_ex_applied"`
	Options      []CartOption `json:"options"`
}
type CartOption struct {
	ID     int         `json:"optionid"`
	Values []CartValue `json:"values"`
}
type CartValue struct {
	ID          int        `json:"valueid"`
	ComboItemID int        `json:"combo_itemid"`
	ComboItems  []CartItem `json:"combo_items"`
}

// ReadCart parses a caller-owned input without storing a cart or history.
func ReadCart(r io.Reader) (Cart, error) {
	var cart Cart
	d := json.NewDecoder(io.LimitReader(r, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(&cart); err != nil {
		return cart, fmt.Errorf("mobileorder: invalid cart: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return cart, errors.New("mobileorder: expected one cart JSON object")
	}
	return cart, cart.validate()
}
func (c Cart) validate() error {
	if _, err := c.Fulfillment.code(); err != nil {
		return err
	}
	if c.LocationID <= 0 || c.PaymentMethod == nil || *c.PaymentMethod < 0 || c.Date == nil || c.Time == nil || len(c.Items) == 0 || c.TenderID < 0 || c.SecondaryTenderID < 0 || c.PickupSpotID < 0 {
		return errors.New("mobileorder: cart requires location, fulfillment, payment_method, date, time and items")
	}
	if *c.PaymentMethod == 0 && c.TenderID <= 0 {
		return errors.New("mobileorder: campus payment requires an explicit tender_id")
	}
	var check func([]CartItem, int) error
	check = func(items []CartItem, depth int) error {
		if depth > 8 {
			return errors.New("mobileorder: combo nesting too deep")
		}
		for _, item := range items {
			if item.ItemID <= 0 || item.SectionID <= 0 {
				return errors.New("mobileorder: item and section IDs must be positive")
			}
			for _, o := range item.Options {
				if o.ID <= 0 {
					return errors.New("mobileorder: option ID must be positive")
				}
				for _, v := range o.Values {
					if v.ID <= 0 || v.ComboItemID < 0 {
						return errors.New("mobileorder: invalid option value")
					}
					if err := check(v.ComboItems, depth+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return check(c.Items, 0)
}
func (c Cart) payload() map[string]any {
	code, _ := c.Fulfillment.code()
	return map[string]any{"locationid": strconv.Itoa(c.LocationID), "retrieval_type": strconv.Itoa(code), "payment_method_default": strconv.Itoa(*c.PaymentMethod), "ct_id": strconv.Itoa(c.TenderID), "ct_id2": strconv.Itoa(c.SecondaryTenderID), "target_date": *c.Date, "target_time": *c.Time, "pickupspotid": strconv.Itoa(c.PickupSpotID), "special_comment": c.Comment, "checkout_select_choiceids": c.CheckoutChoices, "items": c.Items, "os_type": "1", "app_version": "2026.3.1", "app_bundle_name": "com.transact.mobileorder"}
}

// Quote contains server pricing and scheduling details. It does not place an order.
type Quote struct {
	Total int             `json:"total"`
	Cart  json.RawMessage `json:"cart"`
	Date  string          `json:"date"`
	Time  string          `json:"time"`
}

func (c *Client) Quote(ctx context.Context, cart Cart) (*Quote, error) {
	if err := cart.validate(); err != nil {
		return nil, err
	}
	var r struct {
		Cart json.RawMessage `json:"cart"`
		Date string          `json:"target_date"`
		Time string          `json:"target_time"`
	}
	if err := c.call(ctx, "calculatecart", cart.payload(), &r); err != nil {
		return nil, err
	}
	var price struct {
		Total *int `json:"grand_total"`
	}
	if json.Unmarshal(r.Cart, &price) != nil || price.Total == nil || *price.Total < 0 {
		return nil, errors.New("mobileorder: quote has no valid total")
	}
	return &Quote{*price.Total, r.Cart, r.Date, r.Time}, nil
}

// PlaceOrder requotes and compares the exact expected total before submitting once.
// A returned stage is not a payment success until it equals "success".
func (c *Client) PlaceOrder(ctx context.Context, cart Cart, expectedTotal int) (*OrderProgress, error) {
	if expectedTotal < 0 {
		return nil, errors.New("mobileorder: expected total must be nonnegative")
	}
	quote, err := c.Quote(ctx, cart)
	if err != nil {
		return nil, err
	}
	if quote.Total != expectedTotal {
		return nil, fmt.Errorf("mobileorder: total changed to %d cents; review a fresh quote", quote.Total)
	}
	if (quote.Date != "" && quote.Date != *cart.Date) || (quote.Time != "" && quote.Time != *cart.Time) {
		return nil, errors.New("mobileorder: pickup selection changed; review a fresh quote")
	}
	var selection struct {
		Payment   *int    `json:"payment_method_default"`
		Tender    *int    `json:"ct_id"`
		Secondary *int    `json:"ct_id2"`
		Pickup    *int    `json:"pickupspotid"`
		Date      *string `json:"target_date"`
		Time      *string `json:"target_time"`
	}
	if json.Unmarshal(quote.Cart, &selection) != nil || selection.Payment == nil || selection.Tender == nil || selection.Secondary == nil || selection.Pickup == nil || selection.Date == nil || selection.Time == nil {
		return nil, errors.New("mobileorder: quote missing checkout selections")
	}
	if *selection.Payment != *cart.PaymentMethod || *selection.Tender != cart.TenderID || *selection.Secondary != cart.SecondaryTenderID || *selection.Pickup != cart.PickupSpotID || *selection.Date != *cart.Date || *selection.Time != *cart.Time {
		return nil, errors.New("mobileorder: server changed payment or pickup selections; review quote before placing")
	}
	payload := cart.payload()
	var values map[string]json.RawMessage
	if err := json.Unmarshal(quote.Cart, &values); err != nil {
		return nil, err
	}
	for _, key := range []string{"grand_total", "credit_total", "promo_credit_total", "campaign_credit_total", "punchcard_credit_total", "servicefee", "servicefee_tax", "subtotal", "subtotal_tax", "pickup_time_min", "pickup_time_max", "cash_eq_swipes", "meal_ex_swipes"} {
		if v, ok := values[key]; ok {
			var n json.Number
			if json.Unmarshal(v, &n) != nil {
				return nil, errors.New("mobileorder: invalid quoted amount")
			}
			payload[key] = n.String()
		}
	}
	var r progressWire
	if err := c.call(ctx, "processorderstaged", payload, &r); err != nil {
		return nil, &OutcomeUnknown{}
	}
	if r.ID <= 0 || r.Stage == "" {
		return nil, &OutcomeUnknown{ID: r.ID}
	}
	return &OrderProgress{r.ID, r.Stage, r.Message}, nil
}
