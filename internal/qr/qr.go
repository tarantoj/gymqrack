// Package qr renders QR codes as SVG images.
package qr

import (
	go_qr "github.com/piglig/go-qr"
)

// border is the quiet-zone width in modules, matching the previous TypeScript
// server's `qrcode.toBuffer(payload, { width: 512, margin: 2 })`.
const border = 2

// maxSize is the largest rendered side in pixels; larger symbols shrink the
// module scale so the output never exceeds it.
const maxSize = 512

// scaleFor picks a pixels-per-module scale so the full symbol (including the
// quiet zone) fits within maxSize.
func scaleFor(modules int) int {
	if s := maxSize / (modules + 2*border); s > 1 {
		return s
	}
	return 1
}

func encode(payload string) (*go_qr.QrCode, error) {
	return go_qr.EncodeText(payload, go_qr.Medium)
}

// SVG renders payload as a compact single-path SVG with a 2-module quiet zone.
func SVG(payload string) ([]byte, error) {
	q, err := encode(payload)
	if err != nil {
		return nil, err
	}
	return q.ToSVGBytes(go_qr.NewQrCodeImgConfig(scaleFor(q.Size()), border, go_qr.WithOptimalSVG()))
}
