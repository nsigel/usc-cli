// Read enrollments with the sign-in saved by usc auth login.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/nsigel/usc-cli/auth"
	"github.com/nsigel/usc-cli/brightspace"
	"github.com/nsigel/usc-cli/config"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	// Read the credentials usc auth login saved. The session stays in memory;
	// set auth.Options.SessionFile to reuse it across runs.
	credentials, err := config.CredentialsPath()
	if err != nil {
		log.Fatal(err)
	}
	client, err := brightspace.Open(ctx, auth.Options{CredentialsFile: credentials})
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
