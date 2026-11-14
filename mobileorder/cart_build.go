package mobileorder

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// CartSelection supplies explicit choices while menu metadata supplies section
// and option IDs. Repeating an item ID adds another unit with the same choices.
type CartSelection struct {
	Cart    Cart
	ItemIDs []int
	// Choices are ITEM_ID:VALUE_ID. Combo child items use their own item IDs.
	Choices []string
}

// BuildCart resolves item selections against one current menu. It does not quote
// or submit the cart and never selects default modifier values.
func (c *Client) BuildCart(ctx context.Context, s CartSelection) (*Cart, error) {
	cart := s.Cart
	if cart.PaymentMethod == nil || cart.Date == nil || cart.Time == nil || len(s.ItemIDs) == 0 {
		return nil, fmt.Errorf("mobileorder: payment, pickup time and at least one item are required")
	}
	r, err := c.readMenu(ctx, MenuOptions{LocationID: cart.LocationID, Fulfillment: cart.Fulfillment, PaymentMethod: *cart.PaymentMethod, TenderID: cart.TenderID, Date: *cart.Date, Time: *cart.Time})
	if err != nil {
		return nil, err
	}
	choices := map[int][]int{}
	for _, spec := range s.Choices {
		parts := strings.Split(spec, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("mobileorder: choice must be ITEM_ID:VALUE_ID")
		}
		item, e1 := strconv.Atoi(parts[0])
		value, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || item <= 0 || value <= 0 {
			return nil, fmt.Errorf("mobileorder: choice IDs must be positive")
		}
		choices[item] = append(choices[item], value)
	}
	items := map[int]itemWire{}
	sections := map[int]int{}
	top := map[int]bool{}
	for listIndex, list := range [][]sectionWire{r.Menu.Sections, r.Menu.Combos, r.Menu.Extras} {
		for _, section := range list {
			if section.Deleted != 0 {
				continue
			}
			for _, item := range section.Items {
				if item.Deleted != 0 {
					continue
				}
				if _, exists := items[item.ID]; !exists {
					items[item.ID] = item
					sections[item.ID] = section.ID
				}
				if listIndex == 0 && section.Hidden == 0 && item.visible(cart.Fulfillment) {
					top[item.ID] = true
				}
			}
		}
	}
	used := map[int]bool{}
	var build func(int, map[int]bool) (CartItem, error)
	build = func(id int, path map[int]bool) (CartItem, error) {
		item, ok := items[id]
		if !ok {
			return CartItem{}, fmt.Errorf("mobileorder: item %d is not in this menu", id)
		}
		if !item.available(cart.Fulfillment) {
			return CartItem{}, fmt.Errorf("mobileorder: %s is unavailable", item.Name)
		}
		if path[id] || len(path) >= 8 {
			return CartItem{}, fmt.Errorf("mobileorder: recursive combo selection for item %d", id)
		}
		path[id] = true
		defer delete(path, id)
		used[id] = true
		out := CartItem{ItemID: id, SectionID: sections[id], Options: []CartOption{}}
		remaining := map[int]int{}
		for _, v := range choices[id] {
			remaining[v]++
		}
		for _, option := range item.Options {
			if option.Hidden != 0 || option.Deleted != 0 {
				continue
			}
			selected := CartOption{ID: option.ID, Values: []CartValue{}}
			for _, value := range option.Values {
				count := remaining[value.ID]
				if count == 0 {
					continue
				}
				if !value.visible(cart.Fulfillment) || !value.available(cart.Fulfillment) {
					return CartItem{}, fmt.Errorf("mobileorder: %s is unavailable for %s", value.Name, item.Name)
				}
				if count > 1 && option.Quantity != 1 {
					return CartItem{}, fmt.Errorf("mobileorder: %s does not allow repeated choices", option.Name)
				}
				for n := 0; n < count; n++ {
					v := CartValue{ID: value.ID, ComboItemID: value.ComboID, ComboItems: []CartItem{}}
					if value.ComboID != 0 {
						child, err := build(value.ComboID, path)
						if err != nil {
							return CartItem{}, err
						}
						v.ComboItems = append(v.ComboItems, child)
					}
					selected.Values = append(selected.Values, v)
				}
				delete(remaining, value.ID)
			}
			count := len(selected.Values)
			if count < option.Minimum || (option.Maximum > 0 && count > option.Maximum) {
				return CartItem{}, fmt.Errorf("mobileorder: %s requires %d–%d choices for %s; use item details to choose values", option.Name, option.Minimum, option.Maximum, item.Name)
			}
			if count > 0 {
				out.Options = append(out.Options, selected)
			}
		}
		if len(remaining) > 0 {
			return CartItem{}, fmt.Errorf("mobileorder: unknown or hidden modifier value for item %d", id)
		}
		return out, nil
	}
	cart.Items = []CartItem{}
	for _, id := range s.ItemIDs {
		if !top[id] {
			return nil, fmt.Errorf("mobileorder: item %d is not a visible orderable menu item", id)
		}
		item, err := build(id, map[int]bool{})
		if err != nil {
			return nil, err
		}
		cart.Items = append(cart.Items, item)
	}
	for id := range choices {
		if !used[id] {
			return nil, fmt.Errorf("mobileorder: choices supplied for unselected item %d", id)
		}
	}
	if err := cart.validate(); err != nil {
		return nil, err
	}
	return &cart, nil
}
