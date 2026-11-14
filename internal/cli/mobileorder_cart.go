package cli

import (
	"fmt"
	"github.com/nsigel/usc-cli/mobileorder"
	"github.com/spf13/cobra"
)

func mobileBuildCartCommand() *cobra.Command {
	var selection mobileorder.CartSelection
	var fulfillment, date, time string
	var payment int
	var asap bool
	cmd := &cobra.Command{Use: "cart", Short: "Build a cart from item IDs and explicit modifier choices", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		selection.Cart.Fulfillment = mobileorder.Fulfillment(fulfillment)
		selection.Cart.PaymentMethod = &payment
		selection.Cart.Date = &date
		selection.Cart.Time = &time
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		result, err := c.BuildCart(cmd.Context(), selection)
		if err != nil {
			return err
		}
		return writeJSON(cmd, result)
	}}
	f := cmd.Flags()
	f.IntVar(&selection.Cart.LocationID, "location", 0, "location ID")
	f.StringVar(&fulfillment, "fulfillment", "", "pickup, delivery, or address")
	f.IntVar(&payment, "payment-method", 0, "payment method ID from payments")
	f.IntVar(&selection.Cart.TenderID, "tender", 0, "location tender ID from checkout")
	f.IntVar(&selection.Cart.SecondaryTenderID, "secondary-tender", 0, "secondary tender ID when splitting payment")
	f.IntSliceVar(&selection.ItemIDs, "item", nil, "item ID; repeat for multiple items or quantities")
	f.StringArrayVar(&selection.Choices, "select", nil, "ITEM_ID:VALUE_ID modifier choice; repeat for each choice")
	f.BoolVar(&asap, "asap", false, "explicitly request the next available pickup time")
	f.StringVar(&date, "date", "", "scheduled date from checkout")
	f.StringVar(&time, "time", "", "scheduled time from checkout")
	f.IntVar(&selection.Cart.PickupSpotID, "pickup-spot", 0, "pickup spot ID when required")
	f.StringVar(&selection.Cart.Comment, "comment", "", "checkout comment")
	f.IntSliceVar(&selection.Cart.CheckoutChoices, "checkout-choice", nil, "checkout answer ID; repeat for each required answer")
	for _, flag := range []string{"location", "fulfillment", "payment-method", "item"} {
		cmd.MarkFlagRequired(flag)
	}
	cmd.MarkFlagsOneRequired("asap", "date")
	cmd.MarkFlagsRequiredTogether("date", "time")
	cmd.MarkFlagsMutuallyExclusive("asap", "date")
	cmd.MarkFlagsMutuallyExclusive("asap", "time")
	cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("asap") && !asap {
			return fmt.Errorf("--asap must be true, or supply --date and --time")
		}
		return nil
	}
	return cmd
}
