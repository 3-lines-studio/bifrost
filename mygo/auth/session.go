package auth

import (
	"context"
	"sync"

	"github.com/3-lines-studio/bifrost/mygo/client"
)

// Refresher trades the token the server issued for another one, before it
// expires. An app whose server has no such call leaves the session without one,
// and a rejected token signs the app out.
type Refresher func(ctx context.Context, token string) (string, error)

// Session is the signed-in state of the app: the token the server issued, the
// store that keeps it between launches, and the call that renews it. Every
// request the app sends carries the token, and one the server rejects with a
// 401 renews it once before it gives up.
type Session struct {
	mu      sync.RWMutex
	store   Store
	token   string
	refresh Refresher
}

// NewSession reads the token of the last launch from store.
func NewSession(store Store) *Session {
	session := &Session{store: store}
	if store != nil {
		token, err := store.Load()
		if err == nil {
			session.token = token
		}
	}
	return session
}

// WithRefresh makes the session renew a rejected token with refresh.
func (s *Session) WithRefresh(refresh Refresher) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresh = refresh
	return s
}

// Token is the token the session holds, "" when the app is signed out.
func (s *Session) Token() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// SignedIn reports whether the session holds a token.
func (s *Session) SignedIn() bool {
	return s.Token() != ""
}

// Login keeps the token the server issued, and signs the app in.
func (s *Session) Login(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = token
	if s.store == nil {
		return nil
	}
	return s.store.Save(token)
}

// Logout forgets the token, and with it every request the app sends.
func (s *Session) Logout() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token = ""
	if s.store == nil {
		return nil
	}
	return s.store.Clear()
}

// Do sends a request with the token of the session. A 401 renews it once and
// sends the request again; a session that cannot renew it signs out, and the
// app reads the 401 the server answered.
func (s *Session) Do(ctx context.Context, request client.Request) (client.Response, error) {
	token := s.Token()
	response, err := client.Do(ctx, withToken(request, token))
	if err != nil || response.Status != 401 || token == "" {
		return response, err
	}
	s.mu.Lock()
	refresh := s.refresh
	s.mu.Unlock()
	if refresh == nil {
		return response, nil
	}
	renewed, err := refresh(ctx, token)
	if err != nil || renewed == "" {
		_ = s.Logout()
		return response, nil
	}
	if err := s.Login(renewed); err != nil {
		return response, err
	}
	return client.Do(ctx, withToken(request, renewed))
}

// withToken returns the request with the header that carries token, and a copy
// of the headers, so a retry does not send what the last one added.
func withToken(request client.Request, token string) client.Request {
	headers := make(map[string]string, len(request.Header)+1)
	for name, value := range request.Header {
		headers[name] = value
	}
	for name, value := range Header(token) {
		headers[name] = value
	}
	request.Header = headers
	return request
}
