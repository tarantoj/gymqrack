package server

import (
	"embed"
	"html/template"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

var pageTemplates = template.Must(template.ParseFS(templatesFS, "templates/*.html"))

// loginData is the fragment state for the sign-in form.
type loginData struct {
	Error string
}

// qrData is the fragment state for the live QR view. The QR image itself is a
// separate request to /qr/png, so the fragment carries no payload bytes (the
// payload is only ever encoded into the image, never into HTML). Nonce
// cache-busts the image URL so every fragment render forces a fresh fetch
// instead of relying on the browser re-requesting an identical-URL image.
type qrData struct {
	Email   string
	Updated string
	Nonce   int64
}

// pageData is the top-level state for the full page; the embedded view renders
// the login form or the QR view depending on which pointer is set.
type pageData struct {
	Login *loginData
	QR    *qrData
}

// render executes a named template with the given data, setting an HTML Content-Type.
func render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplates.ExecuteTemplate(w, name, data)
}

func renderLogin(w http.ResponseWriter, d loginData) { render(w, "login", d) }

func renderQR(w http.ResponseWriter, d qrData) { render(w, "qr", d) }

func renderPage(w http.ResponseWriter, d pageData) { render(w, "page.html", d) }
