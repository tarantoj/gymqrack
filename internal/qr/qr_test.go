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

func TestSVGDeterministic(t *testing.T) {
	a, _ := SVG("same-payload")
	b, _ := SVG("same-payload")
	if !bytes.Equal(a, b) {
		t.Fatal("SVG output should be deterministic")
	}
}
