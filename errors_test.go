package bifrost

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRedirectRejectsHeaderInjection(t *testing.T) {
	for _, value := range []string{"/safe\r\nX-Test: injected", "/safe\nX-Test: injected"} {
		if IsRedirect(Redirect(value)) {
			t.Fatalf("Redirect(%q) accepted a line break", value)
		}
	}
}

func TestLoaderHTTPResults(t *testing.T) {
	if status, ok := ErrorStatus(NotFound()); !ok || status != http.StatusNotFound {
		t.Fatalf("not found status = %d, %v", status, ok)
	}
	if status, ok := ErrorStatus(Status(http.StatusForbidden, errors.New("denied"))); !ok || status != http.StatusForbidden {
		t.Fatalf("forbidden status = %d, %v", status, ok)
	}
	response := httptest.NewRecorder()
	serveDefaultError(response, httptest.NewRequest(http.MethodGet, "/", nil), Redirect("/login"))
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/login" {
		t.Fatalf("redirect = %d %q", response.Code, response.Header().Get("Location"))
	}
}
