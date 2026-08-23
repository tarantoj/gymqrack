// Package qr renders QR codes as PNG images.
package qr

import "rsc.io/qr"

// border is the quiet-zone width in modules. rsc.io/qr's PNG output always
// frames the symbol with a 4-module white border, matching the VivaGym app's
// ZXing `QRCodeWriter.encode(payload, QR_CODE, 512, 512)` default margin
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

// PNG renders payload as a 1-bit grayscale PNG with a 4-module quiet zone.
func PNG(payload string) ([]byte, error) {
	c, err := qr.Encode(payload, qr.L)
	if err != nil {
		return nil, err
	}
	c.Scale = scaleFor(c.Size)
	return c.PNG(), nil
}
