package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"testing"
)

func flow() Flow {
	return Flow{
		AuthorizeURL: "https://accounts.example.com/authorize",
		ClientID:     "picsel",
		RedirectURI:  "http://127.0.0.1:53682/callback",
		Scopes:       []string{"openid", "email"},
		Extra:        url.Values{"audience": []string{"picsel"}},
	}
}

func TestStart(t *testing.T) {
	request, err := flow().Start()
	if err != nil {
		t.Fatal(err)
	}
	address, err := url.Parse(request.URL)
	if err != nil {
		t.Fatal(err)
	}
	if address.Scheme != "https" || address.Host != "accounts.example.com" || address.Path != "/authorize" {
		t.Fatalf("url %q", request.URL)
	}
	query := address.Query()
	for name, want := range map[string]string{
		"response_type":         "code",
		"client_id":             "picsel",
		"redirect_uri":          "http://127.0.0.1:53682/callback",
		"scope":                 "openid email",
		"state":                 request.State,
		"code_challenge_method": "S256",
		"audience":              "picsel",
	} {
		if got := query.Get(name); got != want {
			t.Fatalf("%s is %q, want %q", name, got, want)
		}
	}
	challenge := sha256.Sum256([]byte(request.Verifier))
	if got := query.Get("code_challenge"); got != base64.RawURLEncoding.EncodeToString(challenge[:]) {
		t.Fatalf("challenge %q", got)
	}
	if len(request.State) < 16 || len(request.Verifier) < 43 {
		t.Fatalf("state %q verifier %q", request.State, request.Verifier)
	}
	second, err := flow().Start()
	if err != nil {
		t.Fatal(err)
	}
	if second.State == request.State || second.Verifier == request.Verifier {
		t.Fatal("two requests share their secrets")
	}
}

func TestStartRejectsBadFlows(t *testing.T) {
	cases := []Flow{
		{AuthorizeURL: "accounts.example.com", ClientID: "a", RedirectURI: "b"},
		{AuthorizeURL: "ftp://accounts.example.com", ClientID: "a", RedirectURI: "b"},
		{AuthorizeURL: "https://accounts.example.com", RedirectURI: "b"},
		{AuthorizeURL: "https://accounts.example.com", ClientID: "a"},
	}
	for _, bad := range cases {
		if _, err := bad.Start(); err == nil {
			t.Fatalf("%+v started", bad)
		}
	}
}

func TestVerify(t *testing.T) {
	request, err := flow().Start()
	if err != nil {
		t.Fatal(err)
	}
	code, err := flow().Verify(url.Values{"code": []string{"the-code"}, "state": []string{request.State}}, request)
	if err != nil {
		t.Fatal(err)
	}
	if code != "the-code" {
		t.Fatalf("code %q", code)
	}
	if _, err := flow().Verify(url.Values{"code": []string{"x"}, "state": []string{"other"}}, request); !errors.Is(err, ErrState) {
		t.Fatalf("another state answered %v", err)
	}
	if _, err := flow().Verify(url.Values{"code": []string{"x"}}, request); !errors.Is(err, ErrState) {
		t.Fatalf("no state answered %v", err)
	}
	if _, err := flow().Verify(url.Values{"state": []string{request.State}}, request); err == nil {
		t.Fatal("no code answered")
	}
	if _, err := flow().Verify(url.Values{"state": []string{request.State}, "error": []string{"access_denied"}}, request); err == nil {
		t.Fatal("an error answered")
	}
}
