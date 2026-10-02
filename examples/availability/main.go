// Query live availability without credentials or the usc executable.
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"time"

	"github.com/nsigel/usc-cli/libcal"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatal(err)
	}
	schedule, err := libcal.NewPublic().Schedule(ctx, libcal.ScheduleOptions{
		Date: time.Now().In(location), Category: libcal.CategoryRooms,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(schedule); err != nil {
		log.Fatal(err)
	}
}
