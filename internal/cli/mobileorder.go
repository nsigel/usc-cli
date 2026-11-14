package cli

import (
	"github.com/nsigel/usc-cli/mobileorder"
	"github.com/spf13/cobra"
	"io"
	"os"
)

func mobileorderCommand() *cobra.Command {
	campus := &cobra.Command{Use: "campus", Short: "Show USC Campus Dining", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		result, err := mobileorder.New().USC(cmd.Context())
		if err != nil {
			return err
		}
		return writeJSON(cmd, result)
	}}
	account := &cobra.Command{Use: "account", Short: "Verify mobile ordering sign-in", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		options, err := signIn()
		if err != nil {
			return err
		}
		client, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		result, err := client.Account(cmd.Context())
		if err != nil {
			return err
		}
		return writeJSON(cmd, result)
	}}
	locations := &cobra.Command{Use: "locations", Short: "List mobile ordering locations", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		options, err := signIn()
		if err != nil {
			return err
		}
		client, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		result, err := client.Locations(cmd.Context())
		if err != nil {
			return err
		}
		return writeJSON(cmd, result)
	}}
	return group("mobileorder", "Browse dining menus and manage mobile orders", campus, account, locations, mobileMenuCommand("menu"), mobileMenuCommand("item"), mobileMenuCommand("checkout"), mobileOrdersCommand(), mobilePaymentsCommand(), mobileBuildCartCommand(), mobileCartCommand(false), group("order", "Place and manage purchases", mobileCartCommand(true), mobileOrderAction("continue"), mobileOrderAction("cancel"), mobileCheckInCommand()))
}

func mobileMenuCommand(kind string) *cobra.Command {
	var fulfillment string
	var o mobileorder.MenuOptions
	cmd := &cobra.Command{Use: kind, Short: map[string]string{"menu": "List menu sections, items and prices", "item": "Show an item and its modifier choices", "checkout": "Show pickup slots, tender rules and checkout questions"}[kind], Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		o.Fulfillment = mobileorder.Fulfillment(fulfillment)
		var r any
		if kind == "item" {
			id, parseErr := positiveID(args[0], "item ID")
			if parseErr != nil {
				return parseErr
			}
			r, err = c.Item(cmd.Context(), o, id)
		} else if kind == "checkout" {
			r, err = c.CheckoutOptions(cmd.Context(), o)
		} else {
			r, err = c.Menu(cmd.Context(), o)
		}
		if err != nil {
			return err
		}
		return writeJSON(cmd, r)
	}}
	cmd.Flags().IntVar(&o.LocationID, "location", 0, "location ID")
	if kind == "item" {
		cmd.Use = "item ITEM_ID"
		cmd.Args = cobra.ExactArgs(1)
	}
	cmd.Flags().StringVar(&fulfillment, "fulfillment", "", "pickup, delivery, or address")
	cmd.Flags().IntVar(&o.PaymentMethod, "payment-method", 0, "Transact payment method")
	cmd.Flags().IntVar(&o.TenderID, "tender", 0, "campus tender ID, when applicable")
	cmd.Flags().StringVar(&o.Date, "date", "", "pickup date")
	cmd.Flags().StringVar(&o.Time, "time", "", "pickup time")
	for _, flag := range []string{"location", "fulfillment", "payment-method"} {
		cmd.MarkFlagRequired(flag)
	}
	return cmd
}
func mobileOrdersCommand() *cobra.Command {
	var pending bool
	cmd := &cobra.Command{Use: "orders", Short: "Read past or pending orders", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		r, err := c.Orders(cmd.Context(), pending)
		if err != nil {
			return err
		}
		return writeJSON(cmd, r)
	}}
	cmd.Flags().BoolVar(&pending, "pending", false, "read pending orders")
	return cmd
}

func mobileCartCommand(place bool) *cobra.Command {
	var file string
	var total int
	use, short := "quote", "Price a cart without placing an order"
	if place {
		use = "place"
		short = "Submit a purchase once at the exact expected total"
	}
	cmd := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		var reader io.Reader = cmd.InOrStdin()
		if file != "-" {
			f, err := os.Open(file)
			if err != nil {
				return err
			}
			defer f.Close()
			reader = f
		}
		cart, err := mobileorder.ReadCart(reader)
		if err != nil {
			return err
		}
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		if place {
			r, err := c.PlaceOrder(cmd.Context(), cart, total)
			if err != nil {
				return err
			}
			return writeJSON(cmd, r)
		}
		r, err := c.Quote(cmd.Context(), cart)
		if err != nil {
			return err
		}
		return writeJSON(cmd, r)
	}}
	cmd.Flags().StringVar(&file, "cart", "", "cart JSON file, or - for stdin")
	cmd.MarkFlagRequired("cart")
	if place {
		cmd.Flags().IntVar(&total, "expected-total", -1, "exact quoted total in cents")
		cmd.MarkFlagRequired("expected-total")
	}
	return cmd
}
func mobileOrderAction(action string) *cobra.Command {
	return &cobra.Command{Use: action + " ORDER_ID", Short: map[string]string{"continue": "Continue staged checkout (may process payment)", "cancel": "Cancel a pending order"}[action], Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := positiveID(args[0], "order ID")
		if err != nil {
			return err
		}
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		if action == "continue" {
			r, err := c.ContinueOrder(cmd.Context(), id)
			if err != nil {
				return err
			}
			return writeJSON(cmd, r)
		}
		r, err := c.CancelOrder(cmd.Context(), id)
		if err != nil {
			return err
		}
		return writeJSON(cmd, r)
	}}
}

func mobilePaymentsCommand() *cobra.Command {
	return &cobra.Command{Use: "payments", Short: "List existing payment methods", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		r, err := c.PaymentMethods(cmd.Context())
		if err != nil {
			return err
		}
		return writeJSON(cmd, r)
	}}
}

func mobileCheckInCommand() *cobra.Command {
	var barcode string
	cmd := &cobra.Command{Use: "check-in ORDER_ID", Short: "Check in for an existing order", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id, err := positiveID(args[0], "order ID")
		if err != nil {
			return err
		}
		options, err := signIn()
		if err != nil {
			return err
		}
		c, err := mobileorder.Open(cmd.Context(), options)
		if err != nil {
			return err
		}
		r, err := c.CheckInOrder(cmd.Context(), id, barcode)
		if err != nil {
			return err
		}
		return writeJSON(cmd, r)
	}}
	cmd.Flags().StringVar(&barcode, "barcode", "", "order check-in barcode")
	cmd.MarkFlagRequired("barcode")
	return cmd
}
