// Package session is the token the demo issues: a stand-in for the real one an
// app would sign.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"strings"
)

const prefix = "demo."

// Issue returns a token for an email.
func Issue(email string) string {
	return prefix + email + "." + random()
}

// Email returns the email a token was issued for, and whether the token is one
// of ours.
func Email(token string) (string, bool) {
	if !strings.HasPrefix(token, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(token, prefix)
	end := strings.LastIndex(rest, ".")
	if end <= 0 {
		return "", false
	}
	return rest[:end], true
}

// Bearer returns the token of an Authorization header, "" for none.
func Bearer(header string) string {
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(token)
}

func random() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buffer)
}
