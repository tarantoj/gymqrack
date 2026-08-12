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

// qrData is the fragment state for the live QR view.
type qrData struct {
	Email   string
	Nonce   int64
	Updated string
	Payload string
}

// pageData is the top-level state for the full page; one of the two embedded
// views is rendered depending on View.
type pageData struct {
	View string
	loginData
	qrData
}

func renderLogin(w http.ResponseWriter, d loginData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplates.ExecuteTemplate(w, "login", d)
}

func renderQR(w http.ResponseWriter, d qrData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplates.ExecuteTemplate(w, "qr", d)
}

func renderPage(w http.ResponseWriter, d pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplates.ExecuteTemplate(w, "page.html", d)
}
