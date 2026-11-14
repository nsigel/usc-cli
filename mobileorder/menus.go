package mobileorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// MenuOptions selects explicit fulfillment and payment context.
type MenuOptions struct {
	LocationID    int
	Fulfillment   Fulfillment
	PaymentMethod int
	TenderID      int
	Date          string
	Time          string
}

// Menu contains visible menu sections and item summaries; prices are cents.
type Menu struct {
	Status     string        `json:"status"`
	LocationID int           `json:"location_id"`
	Sections   []MenuSection `json:"sections"`
}
type MenuSection struct {
	ID        int        `json:"id"`
	Name      string     `json:"name"`
	Available bool       `json:"available"`
	Items     []MenuItem `json:"items"`
}
type MenuItem struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Price     int    `json:"price"`
	Available bool   `json:"available"`
}

// Item includes the selections needed to construct a cart item.
type Item struct {
	MenuItem
	SectionID   int          `json:"section_id"`
	Description string       `json:"description,omitempty"`
	Options     []ItemOption `json:"options"`
}
type ItemOption struct {
	ID            int         `json:"id"`
	Name          string      `json:"name"`
	Minimum       int         `json:"minimum"`
	Maximum       int         `json:"maximum"`
	AllowQuantity bool        `json:"allow_quantity"`
	Values        []ItemValue `json:"values"`
}
type ItemValue struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Price       int    `json:"price"`
	Available   bool   `json:"available"`
	ComboItemID int    `json:"combo_item_id,omitempty"`
}

// CheckoutOptions holds scheduling, tender rules and required checkout questions.
type CheckoutOptions struct {
	LocationID          int             `json:"location_id"`
	Status              string          `json:"status"`
	Instructions        string          `json:"instructions,omitempty"`
	CommentRequired     bool            `json:"comment_required"`
	CommentInstructions string          `json:"comment_instructions,omitempty"`
	Dates               json.RawMessage `json:"dates"`
	Times               json.RawMessage `json:"times"`
	Tenders             json.RawMessage `json:"tenders"`
	PickupSpots         json.RawMessage `json:"pickup_spots"`
	Questions           json.RawMessage `json:"questions"`
}
type menuResponse struct {
	Menu struct {
		Sections []sectionWire   `json:"sections_1"`
		Combos   []sectionWire   `json:"sections_2"`
		Extras   []sectionWire   `json:"sections_3"`
		Tenders  json.RawMessage `json:"tenders"`
	} `json:"menu"`
	Location struct {
		PickupStatus                string          `json:"app_takeout_status_display_text"`
		DeliveryStatus              string          `json:"app_delivery_status_display_text"`
		Instructions                string          `json:"pickup_instructions"`
		CommentRequired             int             `json:"special_comment_required"`
		DeliveryCommentRequired     int             `json:"special_comment_required_delivery"`
		CommentInstructions         string          `json:"special_comment_instructions"`
		DeliveryCommentInstructions string          `json:"special_comment_instructions_delivery"`
		Spots                       json.RawMessage `json:"pickup_spots"`
		Questions                   json.RawMessage `json:"checkout_select_boxes"`
	} `json:"location"`
	Dates json.RawMessage `json:"menu_date_slots"`
	Times json.RawMessage `json:"menu_time_slots"`
}
type sectionWire struct {
	ID        int        `json:"sectionid"`
	Name      string     `json:"name"`
	Hidden    int        `json:"is_hidden"`
	Deleted   int        `json:"is_deleted"`
	Available int        `json:"currently_available"`
	Items     []itemWire `json:"items"`
}
type visibilityWire struct {
	Hidden         int `json:"is_hidden"`
	DeliveryHidden int `json:"is_hidden_delivery"`
	Deleted        int `json:"is_deleted"`
	Online         int `json:"manual_online"`
	DeliveryOnline int `json:"manual_online_delivery"`
	Stock          int `json:"mobile_stock_available"`
}

func (v visibilityWire) visible(f Fulfillment) bool {
	return v.Deleted == 0 && ((f == Pickup && v.Hidden == 0) || (f != Pickup && v.DeliveryHidden == 0))
}
func (v visibilityWire) available(f Fulfillment) bool {
	online := v.Online
	if f != Pickup {
		online = v.DeliveryOnline
	}
	return online == 1 && v.Stock == 1
}

type itemWire struct {
	visibilityWire
	ID          int          `json:"itemid"`
	SectionID   int          `json:"sectionid"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Price       int          `json:"price_display"`
	Options     []optionWire `json:"options"`
}
type optionWire struct {
	ID       int         `json:"optionid"`
	Name     string      `json:"name"`
	Minimum  int         `json:"minimum"`
	Maximum  int         `json:"maximum"`
	Quantity int         `json:"allow_qty"`
	Hidden   int         `json:"is_hidden"`
	Deleted  int         `json:"is_deleted"`
	Values   []valueWire `json:"values"`
}
type valueWire struct {
	visibilityWire
	ID      int    `json:"valueid"`
	Name    string `json:"name"`
	Price   int    `json:"price"`
	ComboID int    `json:"combo_itemid"`
}

func (c *Client) readMenu(ctx context.Context, o MenuOptions) (*menuResponse, error) {
	code, err := o.Fulfillment.code()
	if err != nil {
		return nil, err
	}
	if o.LocationID <= 0 || o.PaymentMethod < 0 || o.PaymentMethod > 2 || o.TenderID < 0 {
		return nil, errors.New("mobileorder: invalid menu selection")
	}
	var r menuResponse
	err = c.call(ctx, "getmenu", map[string]any{"locationid": strconv.Itoa(o.LocationID), "target_date": o.Date, "target_time": o.Time, "retrieval_type": strconv.Itoa(code), "payment_method": strconv.Itoa(o.PaymentMethod), "ct_id": strconv.Itoa(o.TenderID), "ct_id2": "0"}, &r)
	return &r, err
}
func (i itemWire) summary(f Fulfillment, sectionAvailable bool) MenuItem {
	return MenuItem{i.ID, i.Name, i.Price, sectionAvailable && i.available(f)}
}
func (c *Client) Menu(ctx context.Context, o MenuOptions) (*Menu, error) {
	r, err := c.readMenu(ctx, o)
	if err != nil {
		return nil, err
	}
	out := &Menu{LocationID: o.LocationID, Status: r.Location.PickupStatus, Sections: []MenuSection{}}
	if o.Fulfillment != Pickup {
		out.Status = r.Location.DeliveryStatus
	}
	for _, s := range r.Menu.Sections {
		if s.Hidden != 0 || s.Deleted != 0 {
			continue
		}
		section := MenuSection{s.ID, s.Name, s.Available == 1, []MenuItem{}}
		for _, i := range s.Items {
			if i.visible(o.Fulfillment) {
				section.Items = append(section.Items, i.summary(o.Fulfillment, s.Available == 1))
			}
		}
		if len(section.Items) > 0 {
			out.Sections = append(out.Sections, section)
		}
	}
	return out, nil
}
func (c *Client) Item(ctx context.Context, o MenuOptions, id int) (*Item, error) {
	if id <= 0 {
		return nil, errors.New("mobileorder: item ID must be positive")
	}
	r, err := c.readMenu(ctx, o)
	if err != nil {
		return nil, err
	}
	sections := append(append(r.Menu.Sections, r.Menu.Combos...), r.Menu.Extras...)
	for _, s := range sections {
		if s.Deleted != 0 {
			continue
		}
		for _, i := range s.Items {
			if i.ID != id || i.Deleted != 0 {
				continue
			}
			out := &Item{MenuItem: i.summary(o.Fulfillment, s.Available == 1), SectionID: s.ID, Description: i.Description, Options: []ItemOption{}}
			for _, opt := range i.Options {
				if opt.Hidden != 0 || opt.Deleted != 0 {
					continue
				}
				v := ItemOption{opt.ID, opt.Name, opt.Minimum, opt.Maximum, opt.Quantity == 1, []ItemValue{}}
				for _, val := range opt.Values {
					if val.visible(o.Fulfillment) {
						v.Values = append(v.Values, ItemValue{val.ID, val.Name, val.Price, val.available(o.Fulfillment), val.ComboID})
					}
				}
				out.Options = append(out.Options, v)
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("mobileorder: item %d not found in this menu", id)
}
func (c *Client) CheckoutOptions(ctx context.Context, o MenuOptions) (*CheckoutOptions, error) {
	r, err := c.readMenu(ctx, o)
	if err != nil {
		return nil, err
	}
	l := r.Location
	out := &CheckoutOptions{o.LocationID, l.PickupStatus, l.Instructions, l.CommentRequired == 1, l.CommentInstructions, r.Dates, r.Times, r.Menu.Tenders, l.Spots, l.Questions}
	if o.Fulfillment != Pickup {
		out.Status = l.DeliveryStatus
		out.CommentRequired = l.DeliveryCommentRequired == 1
		out.CommentInstructions = l.DeliveryCommentInstructions
	}
	return out, nil
}
