package billing

// createRequest is the JSON body for POST /api/invoices.
type createRequest struct {
	UserID string `json:"userId"`
	Amount int64  `json:"amount"`
}

// chargePayload is the async task payload for billing:charge.
type chargePayload struct {
	InvoiceID string `json:"invoiceId"`
}
