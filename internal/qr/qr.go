// Package qr renders QR codes as SVG images.
package qr

import (
	go_qr "github.com/piglig/go-qr"
)

// border is the quiet-zone width in modules, matching the VivaGym app's ZXing
// `QRCodeWriter.encode(payload, QR_CODE, 512, 512)` default margin
// (`QUIET_ZONE_SIZE = 4`).
const border = 4

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
	// Low error correction matches ZXing's default (QRCodeWriter uses L when no
	// hint is supplied), keeping the rendered pattern identical to the app.
	return go_qr.EncodeText(payload, go_qr.Low)
}

// SVG renders payload as a compact single-path SVG with a 4-module quiet zone,
// matching the VivaGym app's ZXing rendering.
func SVG(payload string) ([]byte, error) {
	q, err := encode(payload)
	if err != nil {
		return nil, err
	}
	return q.ToSVGBytes(go_qr.NewQrCodeImgConfig(scaleFor(q.Size()), border, go_qr.WithOptimalSVG()))
}
