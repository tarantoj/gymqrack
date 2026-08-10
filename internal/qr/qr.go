// Package qr renders QR codes as PNG images.
package qr

import (
	"bytes"
	"image"
	"image/png"

	"github.com/skip2/go-qrcode"
)

// margin is the quiet-zone width in modules, matching the previous TypeScript
// server's `qrcode.toBuffer(payload, { width: 512, margin: 2 })`.
const margin = 2

// PNG renders payload as a 512x512 PNG with a 2-module quiet zone.
func PNG(payload string) ([]byte, error) {
	q, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	q.DisableBorder = true
	bitmap := q.Bitmap()

	modules := len(bitmap)
	total := modules + 2*margin
	scale := 512 / total
	offset := (512 - total*scale) / 2

	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	fill := func(x, y, w, h int, on bool) {
		for yy := y; yy < y+h; yy++ {
			for xx := x; xx < x+w; xx++ {
				if on {
					img.Set(xx, yy, image.Black)
				} else {
					img.Set(xx, yy, image.White)
				}
			}
		}
	}

	// Background (quiet zone + remainder pixels).
	fill(0, 0, 512, 512, false)

	for y, row := range bitmap {
		for x, on := range row {
			fill(offset+(x+margin)*scale, offset+(y+margin)*scale, scale, scale, on)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
