package qr

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("not a valid PNG: %v", err)
	}
	return img
}

// whiteEq reports whether (x, y) is a solid-white quiet-zone pixel.
func whiteEq(img image.Image, x, y int) bool {
	r, g, b, _ := img.At(x, y).RGBA()
	return r == 0xffff && g == 0xffff && b == 0xffff
}

func TestPNGValid(t *testing.T) {
	data, err := PNG("exerp:checkin:test")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	img := decode(t, data)
	b := img.Bounds()
	if b.Dx() != b.Dy() {
		t.Fatalf("expected a square code, got %dx%d", b.Dx(), b.Dy())
	}
	// The 4-module quiet zone borders every edge; every boundary pixel must be
	// white for the code to scan reliably.
	for x := 0; x < b.Dx(); x++ {
		if !whiteEq(img, x, 0) || !whiteEq(img, x, b.Dy()-1) {
			t.Fatalf("quiet zone leaked to the top or bottom edge at x=%d", x)
		}
	}
	for y := 0; y < b.Dy(); y++ {
		if !whiteEq(img, 0, y) || !whiteEq(img, b.Dx()-1, y) {
			t.Fatalf("quiet zone leaked to the left or right edge at y=%d", y)
		}
	}
}

func TestPNGGeometry(t *testing.T) {
	// Pin the rendered geometry: for a version-2 symbol (25 modules) the output
	// is (25 + 2*4) * 15 = 495px square at the maxSize fit of 15px/module with
	// the 4-module quiet zone rsc.io/qr always adds.
	data, err := PNG("exerp:checkin:test")
	if err != nil {
		t.Fatalf("PNG: %v", err)
	}
	img := decode(t, data)
	b := img.Bounds()
	if want := (25 + 2*border) * scaleFor(25); b.Dx() != want || b.Dy() != want {
		t.Fatalf("expected %dx%d, got %dx%d", want, want, b.Dx(), b.Dy())
	}
	if (25+2*border)*scaleFor(25) > maxSize {
		t.Fatal("rendered code exceeds maxSize")
	}
}

func TestPNGDeterministic(t *testing.T) {
	a, _ := PNG("same-payload")
	b, _ := PNG("same-payload")
	if !bytes.Equal(a, b) {
		t.Fatal("PNG output should be deterministic")
	}
}
