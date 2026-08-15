package qr

import (
	"bytes"
	"encoding/xml"
	"testing"
)

type svgRoot struct {
	ViewBox string  `xml:"viewBox,attr"`
	Rect    svgRect `xml:"rect"`
	Path    svgPath `xml:"path"`
}

type svgRect struct {
	Width  string `xml:"width,attr"`
	Height string `xml:"height,attr"`
	Fill   string `xml:"fill,attr"`
}

type svgPath struct {
	D string `xml:"d,attr"`
}

func TestSVGValid(t *testing.T) {
	data, err := SVG("exerp:checkin:test")
	if err != nil {
		t.Fatalf("SVG: %v", err)
	}
	var root svgRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		t.Fatalf("not well-formed XML: %v", err)
	}
	if root.Rect.Width != root.Rect.Height {
		t.Fatalf("expected square background, got %sx%s", root.Rect.Width, root.Rect.Height)
	}
	if root.Path.D == "" {
		t.Fatal("expected a path of dark modules")
	}
	if root.Rect.Fill == "" {
		t.Fatal("expected a light background fill")
	}
}

func TestSVGGeometry(t *testing.T) {
	// Pin the rendered geometry: for a version-2 symbol (25 modules) at the
	// ZXing-matching scale of 15px/module with a 4-module quiet zone, the
	// output is 25*15 + 2*4 = 383px square. Bumping the quiet zone back to 1
	// (or changing the scale) changes this and must fail the test.
	data, err := SVG("exerp:checkin:test")
	if err != nil {
		t.Fatalf("SVG: %v", err)
	}
	var root svgRoot
	if err := xml.Unmarshal(data, &root); err != nil {
		t.Fatalf("not well-formed XML: %v", err)
	}
	// The optimized renderer emits a 100%x100% background rect, so the
	// viewBox is the only geometry that pins the module scale and quiet zone.
	if root.ViewBox != "0 0 383 383" {
		t.Fatalf("expected viewBox %q, got %q", "0 0 383 383", root.ViewBox)
	}
}

func TestSVGDeterministic(t *testing.T) {
	a, _ := SVG("same-payload")
	b, _ := SVG("same-payload")
	if !bytes.Equal(a, b) {
		t.Fatal("SVG output should be deterministic")
	}
}
