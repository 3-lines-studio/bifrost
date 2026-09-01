package notify

// sendRequest is the JSON body for POST /api/notify.
type sendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

// emailPayload is the async task payload for notify:email.
type emailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}
