// Package server puts the graph in front of a browser.
//
// It listens on the loopback address only. A person's coding history is theirs,
// and a viewer for it has no business being reachable from anywhere else on the
// network, so the address is written out rather than left to a wildcard bind.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/graph"
)

// Serve runs bough in the browser until the caller stops it: every project on
// the home page, each one a click away.
//
// start is the path to open first, "/" for the home page or a project's own.
// The address is reported through announce before the browser is opened, so a
// terminal that cannot open one still tells the reader where to look.
func Serve(ctx context.Context, app App, start string, announce func(url string)) error {
	// Port zero asks the operating system for a free one, which avoids both
	// guessing and colliding with whatever else is running.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listening on loopback: %w", err)
	}
	defer func() { _ = listener.Close() }()

	s, err := newSite(app)
	if err != nil {
		return err
	}
	s.refresh()

	ctx, stop := context.WithCancel(ctx)
	defer stop()
	go s.work(ctx)

	host := listener.Addr().String()
	url := "http://" + host + start
	if announce != nil {
		announce(url)
	}
	openBrowser(url)

	srv := &http.Server{
		Handler:           s.routes(host),
		ReadHeaderTimeout: 5 * time.Second,
	}

	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()

	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// pageApp is what a project page needs to know about the bough serving it: the
// way home, and what to send to rename the project. A page saved to disk has
// none of it, and leaves out the controls that would need it.
type pageApp struct {
	Home   string `json:"home"`
	ID     string `json:"id"`
	Token  string `json:"token"`
	Folder string `json:"folder,omitempty"`
}

// render builds the page with the graph already inside it.
//
// Inlining rather than fetching means the page is whole the moment it loads,
// with no second request to wait on, and a saved copy still works after the
// process has gone.
//
// The substitution is done by hand rather than with html/template. There are
// four values, all of them produced here rather than supplied by anyone, and
// the template package drags in reflection and the crypto tree behind its
// contextual escaping. That cost eight megabytes of binary for four
// replacements that need one escaping rule between them.
func render(g graph.Graph, app *pageApp) ([]byte, error) {
	parts := map[string]string{}
	for _, name := range []string{"index.html", "fonts.css", "bough.css", "layout.js", "bough.js"} {
		body, err := readAsset(name)
		if err != nil {
			return nil, err
		}
		parts[name] = body
	}

	data, err := json.Marshal(g)
	if err != nil {
		return nil, fmt.Errorf("encoding the graph: %w", err)
	}
	served, err := json.Marshal(app)
	if err != nil {
		return nil, err
	}

	replace := strings.NewReplacer(
		"{{.Title}}", escapeHTML(g.Project.Name),
		"{{.Fonts}}", parts["fonts.css"],
		"{{.CSS}}", parts["bough.css"],
		"{{.Layout}}", parts["layout.js"],
		"{{.JS}}", parts["bough.js"],
		"{{.Graph}}", escapeScript(string(data)),
		"{{.App}}", escapeScript(string(served)),
	)
	return []byte(replace.Replace(parts["index.html"])), nil
}

// renderHome builds the home page with the list of projects inside it. The
// figures on each card come later, from /api/summaries, since working them
// out reads every history and the page should not wait for that.
func renderHome(cards []Card, token, version string) ([]byte, error) {
	parts := map[string]string{}
	for _, name := range []string{"home.html", "fonts.css", "bough.css", "home.css", "home.js"} {
		body, err := readAsset(name)
		if err != nil {
			return nil, err
		}
		parts[name] = body
	}
	if cards == nil {
		cards = []Card{}
	}
	data, err := json.Marshal(struct {
		Cards   []Card `json:"cards"`
		Token   string `json:"token"`
		Version string `json:"version,omitempty"`
	}{cards, token, version})
	if err != nil {
		return nil, fmt.Errorf("encoding the projects: %w", err)
	}
	replace := strings.NewReplacer(
		"{{.Fonts}}", parts["fonts.css"],
		"{{.CSS}}", parts["bough.css"],
		"{{.HomeCSS}}", parts["home.css"],
		"{{.JS}}", parts["home.js"],
		"{{.Home}}", escapeScript(string(data)),
	)
	return []byte(replace.Replace(parts["home.html"])), nil
}

// escapeHTML makes text safe to drop into the page body. Project names come
// from a directory on this machine, but a name holding a bracket should show
// as that bracket rather than starting a tag.
func escapeHTML(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	).Replace(s)
}

// escapeScript makes JSON safe to sit inside a script element.
//
// JSON is already valid JavaScript, and Go's encoder turns angle brackets into
// escapes on the way out, which handles this on its own today. The rule is
// kept because that behaviour is a setting rather than a guarantee, and people
// do paste markup into these conversations, so a prompt holding the text that
// ends a script element would otherwise cut the page in half.
func escapeScript(s string) string {
	return strings.ReplaceAll(s, "</", `<\/`)
}

func readAsset(name string) (string, error) {
	f, err := assets.Open(name)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	return string(b), err
}

// openBrowser is open, held in a variable so the tests can run a server without
// a browser window appearing for each one.
var openBrowser = open

// open asks the desktop to show a page.
//
// Failure is ignored on purpose. Plenty of places have no browser to open, and
// the caller has already been told the address.
// The url is built from the address the listener bound to, so it is always
// http://127.0.0.1 and a port the kernel chose. It carries nothing a user or
// a transcript supplied, and it is passed as an argument rather than through
// a shell, so there is nothing here for a subprocess to misread.
func open(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url) //#nosec G204
	case "darwin":
		cmd = exec.Command("open", url) //#nosec G204
	default:
		cmd = exec.Command("xdg-open", url) //#nosec G204
	}
	_ = cmd.Start()
}
