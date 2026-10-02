// Read enrollments using an existing session or explicit environment credentials.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := brightspace.OpenWithOptions(ctx, auth.Options{
		Credentials: auth.Credentials{
			Username: os.Getenv("USC_USERNAME"), Password: os.Getenv("USC_PASSWORD"),
			BypassCode: os.Getenv("USC_DUO_BYPASS"),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	courses, err := client.Courses(ctx, false)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(courses); err != nil {
		log.Fatal(err)
	}
}
