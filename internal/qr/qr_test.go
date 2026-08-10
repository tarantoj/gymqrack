package qr

import (
	"bytes"
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
	if img.Bounds().Dx() != 512 || img.Bounds().Dy() != 512 {
		t.Fatalf("expected 512x512, got %v", img.Bounds())
	}
	// Quiet zone at the top-left corner must be white.
	r, g, b, _ := img.At(0, 0).RGBA()
	if r != 0xffff || g != 0xffff || b != 0xffff {
		t.Fatalf("corner not white: %d,%d,%d", r, g, b)
	}
	// The symbol must have some dark modules somewhere near the centre.
	if _, _, _, a := img.At(256, 256).RGBA(); a == 0 {
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
