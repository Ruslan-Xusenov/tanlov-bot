package handlers

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type CAPIEvent struct {
	EventName      string `json:"event_name"`
	Phone          string `json:"phone,omitempty"`
	FirstName      string `json:"first_name,omitempty"`
	LastName       string `json:"last_name,omitempty"`
	EventSourceURL string `json:"event_source_url,omitempty"`
	PixelID        string `json:"pixel_id,omitempty"`
}

// SendCAPIEventAsync sends an event to Meta CAPI asynchronously
func SendCAPIEventAsync(eventName, phone, firstName, lastName string) {
	go func() {
		evt := CAPIEvent{
			EventName:      eventName,
			Phone:          phone,
			FirstName:      firstName,
			LastName:       lastName,
			EventSourceURL: "https://saodatshahri.securehub.uz",
			PixelID:        "1425322323077211",
		}
		data, err := json.Marshal(evt)
		if err != nil {
			return
		}
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post("https://saodatshahri.securehub.uz/api/capi", "application/json", bytes.NewBuffer(data))
		if err != nil {
			log.Printf("[capi] send error: %v", err)
			return
		}
		defer resp.Body.Close()
		log.Printf("[capi] Event %s sent successfully for phone %s (status %d)", eventName, phone, resp.StatusCode)
	}()
}
