package core

import (
	"fmt"
	"net/http"
	"testing"
)

type mockRedirectError struct {
	url    string
	status int
}

func (m *mockRedirectError) Error() string {
	return fmt.Sprintf("redirect to %s", m.url)
}

func (m *mockRedirectError) RedirectURL() string {
	return m.url
}

func (m *mockRedirectError) RedirectStatusCode() int {
	return m.status
}

func TestRedirectErrorInterface(t *testing.T) {
	tests := []struct {
		name         string
		redirectErr  *mockRedirectError
		expectedURL  string
		expectedCode int
	}{
		{
			name:         "custom 301 redirect",
			redirectErr:  &mockRedirectError{url: "/new-path", status: http.StatusMovedPermanently},
			expectedURL:  "/new-path",
			expectedCode: http.StatusMovedPermanently,
		},
		{
			name:         "custom 302 redirect",
			redirectErr:  &mockRedirectError{url: "/temp-path", status: http.StatusFound},
			expectedURL:  "/temp-path",
			expectedCode: http.StatusFound,
		},
		{
			name:         "zero status defaults to 302",
			redirectErr:  &mockRedirectError{url: "/default-path", status: 0},
			expectedURL:  "/default-path",
			expectedCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.redirectErr.RedirectURL() != tt.expectedURL {
				t.Errorf("RedirectURL() = %q, want %q", tt.redirectErr.RedirectURL(), tt.expectedURL)
			}
			if tt.redirectErr.RedirectStatusCode() != tt.expectedCode {
				t.Errorf("RedirectStatusCode() = %d, want %d", tt.redirectErr.RedirectStatusCode(), tt.expectedCode)
			}
		})
	}
}
