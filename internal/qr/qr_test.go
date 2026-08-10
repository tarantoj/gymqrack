package qr

import (
	"bytes"
	"encoding/xml"
	"image/png"
	"testing"
)

func TestPNGValid(t *testing.T) {
	data, err := PNG("exerp:checkin:test")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if img.Bounds().Dx() != img.Bounds().Dy() {
		t.Fatalf("expected square, got %v", img.Bounds())
	}
	if img.Bounds().Dx() > maxSize {
		t.Fatalf("expected <= %dpx, got %v", maxSize, img.Bounds())
	}
	// Quiet zone at the top-left corner must be white.
	r, g, b, _ := img.At(0, 0).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("corner not white: %d,%d,%d", r, g, b)
	}
	// The symbol must have some dark modules somewhere near the centre.
	if _, _, _, a := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA(); a == 0 {
		t.Fatal("center pixel fully transparent")
	}
}

func TestPNGDeterministic(t *testing.T) {
	a, _ := PNG("same-payload")
	b, _ := PNG("same-payload")
	if !bytes.Equal(a, b) {
		t.Fatal("PNG output should be deterministic")
	}
}

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
