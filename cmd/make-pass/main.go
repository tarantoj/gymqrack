// Command make-pass builds a .pkpass for the VivaGym entry QR.
//
// The pass's barcode encodes the URL of the live QR screen (the Wallet pass
// acts as a launcher; iOS does not auto-open URLs from passes).
//
// Signing requires an Apple "Pass Type ID" certificate. Provide PEM material
// via env, otherwise an unsigned zip is emitted (not installable):
//
//	APPLE_TEAM_ID, PASS_TYPE_IDENTIFIER, SIGNING_CERT_PEM, SIGNING_KEY_PEM, WWDR_PEM
package main

import (
	"archive/zip"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func main() {
	publicURL := envOr("PUBLIC_URL", "http://localhost:3000")
	publicURL = strings.TrimRight(publicURL, "/")
	passTypeID := envOr("PASS_TYPE_IDENTIFIER", "pass.com.vivagym.entry")
	teamID := envOr("APPLE_TEAM_ID", "YOUR_TEAM_ID")
	out := envOr("OUT_PASS", filepath.Join("dist", "vivagym.pkpass"))
	qrURL := publicURL + "/qr"

	passJSON := map[string]any{
		"formatVersion":      1,
		"passTypeIdentifier": passTypeID,
		"teamIdentifier":     teamID,
		"serialNumber":       randomHex(16),
		"organizationName":   "VivaGym",
		"description":        "VivaGym gym entry QR",
		"logoText":           "VivaGym",
		"foregroundColor":    "rgb(255,255,255)",
		"backgroundColor":    "rgb(253,80,0)",
		"labelColor":         "rgb(0,0,0)",
		"barcode": map[string]string{
			"format":          "PKBarcodeFormatQR",
			"message":         qrURL,
			"messageEncoding": "utf-8",
		},
		"storeCard": map[string]any{
			"primaryFields": []map[string]string{
				{"key": "entry", "label": "Entrada / Access", "value": "Abrir QR / Open QR"},
			},
			"auxiliaryFields": []map[string]string{
				{"key": "refresh", "label": "Actualización", "value": "Se renueva cada minuto"},
			},
			"backFields": []any{
				map[string]string{"key": "url", "label": "QR URL", "value": qrURL},
				map[string]string{
					"key":   "usage",
					"label": "Uso / Usage",
					"value": "iOS no abre URLs automáticamente desde el pase. Abre la URL (o esta web) y muestra el QR en pantalla para el torno. / iOS does not open URLs from a pass automatically. Open the URL and show the QR on screen at the turnstile.",
				},
			},
		},
	}

	files := map[string][]byte{}
	passBytes, _ := json.MarshalIndent(passJSON, "", "  ")
	files["pass.json"] = passBytes
	orange := color.RGBA{R: 253, G: 80, B: 0, A: 255}
	files["icon.png"] = solidPNG(29, orange)
	files["icon@2x.png"] = solidPNG(58, orange)
	files["logo.png"] = solidPNG(58, orange)
	files["logo@2x.png"] = solidPNG(116, orange)

	manifest := map[string]string{}
	for name, content := range files {
		sum := sha1.Sum(content)
		manifest[name] = hex.EncodeToString(sum[:])
	}
	manifestBytes, _ := json.Marshal(manifest)
	files["manifest.json"] = manifestBytes

	if cert := os.Getenv("SIGNING_CERT_PEM"); cert != "" {
		if key := os.Getenv("SIGNING_KEY_PEM"); key != "" {
			dir, err := os.MkdirTemp("", "vivagym-pass-")
			if err != nil {
				log.Fatal(err)
			}
			defer os.RemoveAll(dir)
			mustWrite(dir, "cert.pem", []byte(cert))
			mustWrite(dir, "key.pem", []byte(key))
			if wwdr := os.Getenv("WWDR_PEM"); wwdr != "" {
				mustWrite(dir, "wwdr.pem", []byte(wwdr))
			}
			args := []string{
				"smime", "-sign", "-binary",
				"-in", "manifest.json",
				"-out", "signature",
				"-signer", "cert.pem",
				"-inkey", "key.pem",
				"-outform", "DER",
				"-noattr",
			}
			if os.Getenv("WWDR_PEM") != "" {
				args = append(args, "-certfile", "wwdr.pem")
			}
			cmd := exec.Command("openssl", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				log.Fatalf("openssl smime sign failed: %v\n%s", err, out)
			}
			sig, err := os.ReadFile(filepath.Join(dir, "signature"))
			if err != nil {
				log.Fatal(err)
			}
			files["signature"] = sig
			log.Println("Signed signature with provided certificate.")
		}
	} else {
		log.Println("WARNING: no signing cert (SIGNING_CERT_PEM/SIGNING_KEY_PEM/WWDR_PEM). The .pkpass will be UNSIGNED and cannot be installed in Wallet.")
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			log.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			log.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		log.Fatal(err)
	}

	log.Printf("Pass written to %s", out)
	log.Printf("Barcode message (QR URL): %s", qrURL)
}

func solidPNG(size int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, c)
		}
	}
	var buf strings.Builder
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return []byte(buf.String())
}

func mustWrite(dir, name string, content []byte) {
	if err := os.WriteFile(filepath.Join(dir, name), content, 0o600); err != nil {
		log.Fatal(err)
	}
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	if _, err := rand.Read(b); err != nil {
		log.Fatal(err)
	}
	return hex.EncodeToString(b)
}
