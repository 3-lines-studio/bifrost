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
	"strconv"
	"strings"
	"sync"

	"github.com/3-lines-studio/bifrost/mygo/auth"
	"github.com/3-lines-studio/bifrost/mygo/client"
	uploadpage "github.com/3-lines-studio/bifrost/mygo/demo/app/upload"
	"github.com/3-lines-studio/bifrost/mygo/demo/store"
	"github.com/3-lines-studio/bifrost/mygo/demo/uploadproto"
	"github.com/3-lines-studio/bifrost/mygo/exif"
	"github.com/3-lines-studio/bifrost/mygo/file"
	"github.com/3-lines-studio/bifrost/mygo/qr"
	"github.com/3-lines-studio/bifrost/mygo/upload"
	"github.com/3-lines-studio/bifrost/mygo/web"
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

	uploads   []uploadItem
	uploadErr string
}

// uploadItem is one file of the upload screen, and the key of its upload in the
// store, as the name and the size of the file make it.
type uploadItem struct {
	key  string
	item uploadpage.Item
}

func main() {
	web.FragmentLocation()
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

// Choose asks the person for files and sends them: the picker and the upload
// wait for the person and for the server, so they run off the thread that
// draws.
func (a *app) Choose() {
	go a.choosePhotos()
}

func (a *app) choosePhotos() {
	ctx := context.Background()
	picked, err := file.Pick(ctx, file.Options{
		Title:    "Photos",
		Multiple: true,
		Filters:  []mygo.FileFilter{{Name: "Images", MIMETypes: []string{"image/*"}}},
	})
	if err != nil {
		a.failUploads(err)
		return
	}
	a.setUploadErr("")
	signer := uploadproto.NewSigner(a.base, appCall{a})
	for _, picture := range picked {
		a.sendFile(ctx, signer, picture)
		picture.Close()
	}
}

// sendFile reads what the picture says of itself, sends it in parts, and shows
// the link and the code of the file the server joined.
func (a *app) sendFile(ctx context.Context, signer upload.Signer, picture *file.File) {
	key := picture.Name() + ":" + strconv.FormatInt(picture.Size(), 10)
	a.addUpload(uploadItem{key: key, item: uploadpage.Item{Name: picture.Name(), Size: picture.Size()}})
	if head, err := picture.Head(ctx, exif.HeaderSize); err == nil {
		if metadata, err := exif.ReadBytes(head); err == nil {
			a.updateUpload(key, func(item *uploadpage.Item) { item.Note = describe(metadata) })
			a.repaint()
		}
	}
	result, err := upload.Run(ctx, picture, signer, upload.Options{
		Key:   key,
		Store: uploadStore(),
		OnProgress: func(progress upload.Progress) {
			a.updateUpload(key, func(item *uploadpage.Item) { item.Sent = progress.Sent })
			a.repaint()
		},
	})
	if err != nil {
		a.updateUpload(key, func(item *uploadpage.Item) { item.Err = err.Error() })
		a.repaint()
		return
	}
	link := publicURL() + "/api/media/" + result.UploadID
	code, err := qr.Encode(link, qr.Medium)
	a.updateUpload(key, func(item *uploadpage.Item) {
		if err != nil {
			item.Err = err.Error()
			return
		}
		item.URL, item.Code, item.Sent = link, code, item.Size
	})
	a.repaint()
}

// describe is what a picture says of itself, in one line of the screen.
func describe(metadata exif.Metadata) string {
	parts := make([]string, 0, 5)
	if width, height := metadata.Rotated(); width > 0 {
		parts = append(parts, strconv.Itoa(width)+"×"+strconv.Itoa(height))
	}
	if camera := strings.TrimSpace(metadata.Make + " " + metadata.Model); camera != "" {
		parts = append(parts, camera)
	}
	if metadata.ISO > 0 {
		parts = append(parts, "ISO "+strconv.Itoa(metadata.ISO))
	}
	if metadata.FNumber > 0 {
		parts = append(parts, "f/"+strconv.FormatFloat(metadata.FNumber, 'f', -1, 64))
	}
	if !metadata.Captured.IsZero() {
		parts = append(parts, metadata.Captured.Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, " · ")
}

// Uploads is what the upload screen shows.
func (a *app) Uploads() []uploadpage.Item {
	a.mu.Lock()
	defer a.mu.Unlock()
	items := make([]uploadpage.Item, 0, len(a.uploads))
	for _, held := range a.uploads {
		items = append(items, held.item)
	}
	return items
}

// UploadErr is what the picker failed with.
func (a *app) UploadErr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.uploadErr
}

// appCall is the app's call, as the upload protocol asks for it.
type appCall struct{ app *app }

func (c appCall) Call(ctx context.Context, method, path string, input, output any) error {
	return c.app.call(ctx, method, path, input, output)
}

func (a *app) addUpload(item uploadItem) {
	a.mu.Lock()
	a.uploads = append(a.uploads, item)
	a.mu.Unlock()
	a.repaint()
}

func (a *app) updateUpload(key string, change func(*uploadpage.Item)) {
	a.mu.Lock()
	for index := range a.uploads {
		if a.uploads[index].key == key {
			change(&a.uploads[index].item)
			break
		}
	}
	a.mu.Unlock()
}

func (a *app) failUploads(err error) {
	a.mu.Lock()
	a.uploadErr = err.Error()
	a.mu.Unlock()
	a.repaint()
}

func (a *app) setUploadErr(message string) {
	a.mu.Lock()
	a.uploadErr = message
	a.mu.Unlock()
}

// repaint asks the window for a frame after a goroutine changed what the screen
// shows.
func (a *app) repaint() {
	if a.win != nil {
		a.win.Update(func() {})
	}
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
