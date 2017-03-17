package analytics

import (
	"encoding/json"
	"os"
	"time"

	analytics "github.com/segmentio/analytics-go"
)

// ReportAnalyticsEvent is the main function when docker is executed only
// for reporting a tracking event
func ReportAnalyticsEvent() {
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
		err = eventDirect(&event)
		if err != nil {
			return
		}
	} else {
		// number of arguments is incorrect
		return
	}
}
