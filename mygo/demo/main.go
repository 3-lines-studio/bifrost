// Command demo is one Go source for the server, the browser and the desktop:
// the pages under app/ are rendered to HTML for a crawler, compiled to
// WebAssembly for a browser, and drawn in the window of a MyGo app.
//
//	go run .                     # the desktop app, against the server
//	bifrost dev                  # the server, with the browser build beside it
//	bifrost build && ./.bifrost/bifrost-app
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/3-lines-studio/bifrost/mygo/auth"
	"github.com/3-lines-studio/bifrost/mygo/client"
	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// app is what the screens read and change: the client of the server, the
// session the token lives in, and what the last request answered.
type app struct {
	base    string
	session *auth.Session
	router  *ui.Router
	win     *mygo.Window

	// The form of the login screen belongs to the UI thread, which reads and
	// changes it inside a frame: no lock.
	email    string
	password string

	mu      sync.Mutex
	notes   []store.Note
	profile string
	notice  string
	loadErr string
}

func main() {
	instance := &app{base: serverURL(), router: ui.NewRouter("/")}
	instance.session = auth.NewSession(auth.System()).WithRefresh(instance.renew)
	mygo.App.WhenReady(func() {
		instance.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "Bifrost + MyGo",
			Width:   980,
			Height:  760,
			Content: ui.View(instance.view),
		})
		instance.Refresh()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// view draws the page of the path, from the route table bifrost build
// generates out of the Screen functions of the pages.
func (a *app) view(c *ui.Context) {
	a.router.View(c, func(r *ui.Route) {
		for _, page := range appPages(a) {
			if r.Match(page.Pattern) {
				page.View(c)
				return
			}
		}
		ui.Text(c, "This path is not a screen of the app.")
	})
}

// call sends a request to the server with the token of the session.
func (a *app) call(ctx context.Context, method, path string, input, output any) error {
	header := map[string]string{}
	var body []byte
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body, header["Content-Type"] = encoded, "application/json"
	}
	response, err := a.session.Do(ctx, client.Request{Method: method, URL: a.base + path, Header: header, Body: body})
	if err != nil {
		return err
	}
	if response.Status < 200 || response.Status > 299 {
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(response.Body, &failure) == nil && failure.Error != "" {
			return errors.New(failure.Error)
		}
		return fmt.Errorf("%s %s answered %d", method, path, response.Status)
	}
	if output == nil {
		return nil
	}
	return json.Unmarshal(response.Body, output)
}

// SignIn trades the credentials of the login screen for a token.
func (a *app) SignIn() {
	email, password := a.email, a.password
	if email == "" || password == "" {
		a.setNotice("Type an email and a password.")
		return
	}
	go func() {
		var answer struct {
			Token string `json:"token"`
		}
		err := a.call(context.Background(), "POST", "/api/login", map[string]string{"email": email, "password": password}, &answer)
		if err == nil {
			err = a.session.Login(answer.Token)
		}
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if err != nil {
				a.setNotice(err.Error())
				return
			}
			a.setNotice("Signed in as " + email + ".")
			a.Refresh()
		})
	}()
}

// SignOut forgets the token and what it read.
func (a *app) SignOut() {
	go func() {
		err := a.session.Logout()
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			a.setProfile("")
			a.setNotes(nil)
			if err != nil {
				a.setNotice(err.Error())
				return
			}
			a.setNotice("Signed out.")
		})
	}()
}

// Refresh loads the notes again. The request runs off the thread that draws,
// because a request blocks until the browser answers it and the thread that
// draws is the one the answer arrives on: blocking it would wait forever.
func (a *app) Refresh() {
	go func() {
		var notes []store.Note
		err := a.call(context.Background(), "GET", "/api/notes", nil, &notes)
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if err != nil {
				a.setLoadError(err.Error())
				return
			}
			a.setLoadError("")
			a.setNotes(notes)
		})
	}()
}

// LoadProfile reads the endpoint that only a signed-in app reaches.
func (a *app) LoadProfile() {
	go func() {
		var profile struct {
			Email string `json:"email"`
		}
		err := a.call(context.Background(), "GET", "/api/me", nil, &profile)
		if a.win == nil {
			return
		}
		a.win.Update(func() {
			if err != nil {
				a.setProfile("")
				a.setNotice(err.Error())
				return
			}
			a.setProfile(profile.Email)
			a.setNotice("")
		})
	}()
}

// renew trades a token the server rejected for another one.
func (a *app) renew(ctx context.Context, token string) (string, error) {
	var answer struct {
		Token string `json:"token"`
	}
	header := map[string]string{"Authorization": "Bearer " + token}
	response, err := client.Do(ctx, client.Request{Method: "POST", URL: a.base + "/api/refresh", Header: header})
	if err != nil {
		return "", err
	}
	if response.Status != 200 {
		return "", fmt.Errorf("refresh answered %d", response.Status)
	}
	err = json.Unmarshal(response.Body, &answer)
	return answer.Token, err
}

// Notes is what the last load read.
func (a *app) Notes() []store.Note {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]store.Note(nil), a.notes...)
}

// LoadErr is what the last load failed with.
func (a *app) LoadErr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loadErr
}

// Profile is the email of the signed-in user the server answered with.
func (a *app) Profile() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.profile
}

// Notice is what the last action said.
func (a *app) Notice() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.notice
}

// Path is the path the router shows, which the screens that have a parameter
// read.
func (a *app) Path() string { return a.router.Path() }

// SignedIn reports whether the app holds a token.
func (a *app) SignedIn() bool { return a.session.SignedIn() }

// Email is the login screen's field, which the UI thread owns.
func (a *app) Email() *string { return &a.email }

// Password is the login screen's field, which the UI thread owns.
func (a *app) Password() *string { return &a.password }

func (a *app) setNotes(notes []store.Note) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notes = notes
}

func (a *app) setProfile(profile string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.profile = profile
}

func (a *app) setNotice(notice string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.notice = notice
}

func (a *app) setLoadError(err string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.loadErr = err
}
