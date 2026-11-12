// Read the WebReg course bin using an existing session or explicit environment credentials.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/webreg"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := webreg.OpenWithOptions(ctx, auth.Options{
		Credentials: auth.Credentials{
			Username: os.Getenv("USC_USERNAME"), Password: os.Getenv("USC_PASSWORD"),
			BypassCode: os.Getenv("USC_DUO_BYPASS"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	terms, err := client.Terms(ctx)
	if err != nil {
		log.Fatal(err)
	}
	bin, err := client.CourseBin(ctx, terms[0].Code)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(bin); err != nil {
		log.Fatal(err)
	}
}
