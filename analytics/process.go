package analytics

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	analytics "github.com/segmentio/analytics-go"
)

// ReportAnalyticsEvent is the main function when docker is executed only
// for reporting a tracking event
func ReportAnalyticsEvent() {
	// create analytics client
	var client *analytics.Client = analytics.New("EMkyNVNnr7Ian1RrSOW8b4JdAt4GQ7lI")

	// defer time.Sleep(10 * time.Second)
	defer client.Close()
	defer time.Sleep(60 * time.Second)

	// retrieve arguments
	args := os.Args
	if len(args) == 2 {
		eventJSON := args[1]
		var event analytics.Track
		err := json.Unmarshal([]byte(eventJSON), &event)
		if err != nil {
			return
		}

		// client.Verbose = true
		client.Size = 1
		// identify users that are logged in
		client.Identify(&analytics.Identify{
			UserId: event.UserId,
			Traits: map[string]interface{}{
				"login": event.Properties["username"],
			},
		})

		err = client.Track(&event)
		if err != nil {
			fmt.Println("process track error:", err.Error())
			return
		}
	} else {
		// number of arguments is incorrect
		return
	}
}
