package mobileorder

import "fmt"

// Fulfillment identifies how an order is received.
type Fulfillment string

const (
	Pickup   Fulfillment = "pickup"
	Delivery Fulfillment = "delivery"
	Address  Fulfillment = "address"
)

func (f Fulfillment) code() (int, error) {
	switch f {
	case Pickup:
		return 0, nil
	case Delivery:
		return 1, nil
	case Address:
		return 2, nil
	}
	return 0, fmt.Errorf("mobileorder: fulfillment must be pickup, delivery, or address")
}
