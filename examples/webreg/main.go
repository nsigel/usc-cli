package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	usc "github.com/nsigel/usc-cli"
	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/webreg"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	credentials, err := usc.CredentialsPath()
	if err != nil {
		log.Fatal(err)
	}
	client, err := webreg.Open(ctx, auth.Options{CredentialsFile: credentials})
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
