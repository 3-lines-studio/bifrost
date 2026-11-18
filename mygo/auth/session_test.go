package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3-lines-studio/bifrost/mygo/client"
)

type memory struct {
	token   string
	saved   int
	cleared int
}

func (m *memory) Load() (string, error) { return m.token, nil }

func (m *memory) Save(token string) error {
	m.token, m.saved = token, m.saved+1
	return nil
}

func (m *memory) Clear() error {
	m.token, m.cleared = "", m.cleared+1
	return nil
}

func TestSessionReadsAndKeepsTheToken(t *testing.T) {
	store := &memory{token: "first"}
	session := NewSession(store)
	if !session.SignedIn() || session.Token() != "first" {
		t.Fatalf("session %+v", session)
	}
	if err := session.Login("second"); err != nil {
		t.Fatal(err)
	}
	if store.token != "second" {
		t.Fatalf("store holds %q", store.token)
	}
	if err := session.Logout(); err != nil {
		t.Fatal(err)
	}
	if session.SignedIn() || store.token != "" {
		t.Fatalf("session %+v store %+v", session, store)
	}
}

func TestSessionSendsTheToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	session := NewSession(&memory{token: "secret"})
	response, err := session.Do(context.Background(), client.Request{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("status %d", response.Status)
	}
}

func TestSessionRenewsOnce(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		seen = append(seen, token)
		if token != "Bearer renewed" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	store := &memory{token: "stale"}
	session := NewSession(store).WithRefresh(func(context.Context, string) (string, error) {
		return "renewed", nil
	})
	response, err := session.Do(context.Background(), client.Request{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusOK {
		t.Fatalf("status %d", response.Status)
	}
	if got := store.token; got != "renewed" {
		t.Fatalf("store holds %q", got)
	}
	if len(seen) != 2 {
		t.Fatalf("the server saw %v", seen)
	}
}

func TestSessionSignsOutWhenTheRefreshFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	store := &memory{token: "stale"}
	session := NewSession(store).WithRefresh(func(context.Context, string) (string, error) {
		return "", errors.New("the server said no")
	})
	response, err := session.Do(context.Background(), client.Request{URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if response.Status != http.StatusUnauthorized {
		t.Fatalf("status %d", response.Status)
	}
	if session.SignedIn() || store.cleared != 1 {
		t.Fatalf("session %+v store %+v", session, store)
	}
}

func TestSessionKeepsARejectedTokenWithoutRefresh(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	session := NewSession(&memory{token: "stale"})
	if _, err := session.Do(context.Background(), client.Request{URL: server.URL}); err != nil {
		t.Fatal(err)
	}
	if !session.SignedIn() {
		t.Fatal("a session without a refresh signed out")
	}
}
