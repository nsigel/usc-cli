package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/nsigel/usc-cli/classes"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client := classes.New()
	terms, err := client.Terms(ctx)
	if err != nil {
		log.Fatal(err)
	}
	schools, err := client.Schools(ctx, terms[0].Code)
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(schools); err != nil {
		log.Fatal(err)
	}
}
