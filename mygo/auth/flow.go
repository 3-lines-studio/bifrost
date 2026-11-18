package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/egoist/mygo"
)

// ErrState is what Verify answers for an answer of another request: one a
// stranger started, or a replay of an old one.
var ErrState = errors.New("auth: the answer belongs to another request")

// Flow is the authorization code flow of OAuth 2.0 with PKCE, as the servers
// that sign people in take it: the app opens the page of the provider, the
// person signs in there, and the server sends the browser back with a code the
// app exchanges for a token.
//
//	callback, err := auth.Listen("127.0.0.1:0")
//	flow := auth.Flow{
//		AuthorizeURL: "https://accounts.example.com/authorize",
//		ClientID:     "picsel",
//		RedirectURI:  callback.URL(),
//		Scopes:       []string{"openid", "email"},
//	}
//	request, err := flow.Start()
//	auth.Open(request.URL)
//	answer, err := callback.Wait(ctx)
//	code, err := flow.Verify(answer, request)
type Flow struct {
	// AuthorizeURL is the page of the provider people sign in on.
	AuthorizeURL string
	ClientID     string
	// RedirectURI is where the browser comes back: the URL of a Callback in a
	// window, and the address of the app's own handler in a browser.
	RedirectURI string
	Scopes      []string
	// Extra are the values the provider wants beside the ones of the flow, as
	// "audience". They never replace them.
	Extra url.Values
}

// Request is what Start asks for: the page the browser opens, and the secrets
// Verify checks the answer with.
type Request struct {
	URL      string
	State    string
	Verifier string
}

// Start returns the request of a sign-in, with a state and a proof key of its
// own, as PKCE asks.
func (f Flow) Start() (Request, error) {
	address, err := url.Parse(f.AuthorizeURL)
	if err != nil || address.Host == "" || (address.Scheme != "https" && address.Scheme != "http") {
		return Request{}, errors.New("auth: AuthorizeURL must be an absolute http or https URL")
	}
	if f.ClientID == "" {
		return Request{}, errors.New("auth: no ClientID")
	}
	if f.RedirectURI == "" {
		return Request{}, errors.New("auth: no RedirectURI")
	}
	verifier, err := secret(32)
	if err != nil {
		return Request{}, err
	}
	state, err := secret(16)
	if err != nil {
		return Request{}, err
	}
	challenge := sha256.Sum256([]byte(verifier))
	query := url.Values{}
	for name, values := range f.Extra {
		query[name] = slices.Clone(values)
	}
	query.Set("response_type", "code")
	query.Set("client_id", f.ClientID)
	query.Set("redirect_uri", f.RedirectURI)
	if len(f.Scopes) > 0 {
		query.Set("scope", strings.Join(f.Scopes, " "))
	}
	query.Set("state", state)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:]))
	query.Set("code_challenge_method", "S256")
	address.RawQuery = query.Encode()
	return Request{URL: address.String(), State: state, Verifier: verifier}, nil
}

// Verify returns the code of the answer the provider sent back, and checks that
// it belongs to request.
func (f Flow) Verify(answer url.Values, request Request) (string, error) {
	if answer.Get("state") != request.State || request.State == "" {
		return "", ErrState
	}
	if message := answer.Get("error"); message != "" {
		if description := answer.Get("error_description"); description != "" {
			return "", fmt.Errorf("auth: %s: %s", message, description)
		}
		return "", fmt.Errorf("auth: %s", message)
	}
	code := answer.Get("code")
	if code == "" {
		return "", errors.New("auth: the provider sent no code")
	}
	return code, nil
}

// Open opens the page of the provider: the default browser of the system, or a
// tab of the one the app runs in.
func Open(address string) error {
	return mygo.Shell.OpenExternal(address)
}

// secret returns how many random bytes as a string of the URL safe alphabet.
func secret(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
