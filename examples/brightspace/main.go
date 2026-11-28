package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	usc "github.com/nsigel/usc-cli"
	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	credentials, err := usc.CredentialsPath()
	if err != nil {
		log.Fatal(err)
	}
	client, err := brightspace.Open(ctx, auth.Options{CredentialsFile: credentials})
	if err != nil {
		log.Fatal(err)
	}
	courses, err := client.Courses(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(courses); err != nil {
		log.Fatal(err)
	}
}
