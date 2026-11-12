package main

import (
	"context"
	"encoding/json"
	"github.com/nsigel/usc-cli/classes"
	"os"
)

func main() {
	ctx := context.Background()
	client := classes.New()
	terms, err := client.Terms(ctx)
	if err != nil {
		panic(err)
	}
	if len(terms) == 0 {
		return
	}
	schools, err := client.Schools(ctx, terms[0].TermCode)
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(schools); err != nil {
		panic(err)
	}
}
