// Package login is the sign-in screen: the app draws it, and no crawler reads
// it, so it has no View and the server answers a shell nobody indexes.
package login

import (
	"github.com/3-lines-studio/bifrost/mygo/seo"
	"github.com/egoist/mygo/ui"
)

// Deps is what the app gives the screen: the fields the UI thread owns, the
// call that trades them for a token, and what the last one said.
type Deps interface {
	Email() *string
	Password() *string
	SignIn()
	Notice() string
	SignedIn() bool
}

// Screen draws the form.
func Screen(deps Deps) func(*ui.Context) {
	return func(c *ui.Context) {
		ui.Column(c).Padding(28).Gap(14).Children(func() {
			ui.Text(c, "Sign in").FontSize(32).Bold().Role(ui.RoleHeading).Level(1)
			ui.Text(c, "The server issues a token. The app keeps it in the keychain, or in localStorage in a browser, and sends it with every request.")
			ui.Form(c, func() {
				ui.Field(c, "Email", func() {
					ui.TextInput(c, deps.Email()).Placeholder("you@example.com")
				})
				ui.Field(c, "Password", func() {
					ui.TextInput(c, deps.Password()).Password()
				})
				if ui.PrimaryButton(c, "Sign in").Clicked() {
					deps.SignIn()
				}
			})
			if notice := deps.Notice(); notice != "" {
				ui.Text(c, notice)
			}
			switch {
			case deps.SignedIn():
				ui.Text(c, "Signed in.")
				seo.Link(c, "Open the dashboard", "/dashboard")
			default:
				ui.Text(c, "Any password of four letters or more signs in.")
			}
		})
	}
}

// Metadata is the title of the tab, which is not indexed.
func Metadata() seo.Head {
	return seo.Head{Title: "Sign in · Bifrost + MyGo", Robots: "noindex"}
}
