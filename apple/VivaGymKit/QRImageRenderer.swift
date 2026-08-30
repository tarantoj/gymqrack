import CoreGraphics
import UIKit

/// Renders a VivaGym QR payload to a bitmap.
///
/// The payload is opaque server-signed text (`exerp:checkin:<memberRef>-<millis>-<digest>`)
/// that must be passed through unchanged. Encoding uses the Nayuki pure-Swift
/// generator at error-correction level **L** (reproducing the ZXing default and
/// gymqrack's `rsc.io/qr`, both of which hard-code level L), padded with a
/// 4-module quiet zone.
///
/// CoreImage is deliberately not used: its `CIQRCodeGenerator` is unavailable on
/// watchOS, so this shared renderer uses a Foundation-only encoder that builds on
/// both watchOS and iOS.
public enum QRImageRenderer {
    public static func makeImage(
        payload: String,
        correctionLevel: String = "L",
        quietModules: Int = 4,
        minSidePixels: Int = 1024
    ) -> UIImage? {
        guard !payload.isEmpty else { return nil }
        let code: QRCode
        do {
            code = try QRCode.encode(
                segments: QRSegment.makeSegments(Array(payload)),
                ecl: Self.ecc(for: correctionLevel),
                boostECL: false
            )
        } catch {
            return nil
        }

        let modules = code.size
        guard modules > 1 else { return nil }

        let side = modules + quietModules * 2
        let scale = max(2, Int(ceil(Double(minSidePixels) / Double(side))))
        let pixelSide = CGFloat(side * scale)

        #if os(watchOS)
        // UIGraphicsImageRenderer is unavailable on watchOS; the C image-context
        // pair is deprecated but present (watchOS 2+). Opaque so the modules
        // stay solid black instead of rendering with a transparent alpha.
        UIGraphicsBeginImageContextWithOptions(CGSize(width: pixelSide, height: pixelSide), true, 1)
        defer { UIGraphicsEndImageContext() }
        guard let cg = UIGraphicsGetCurrentContext() else { return nil }
        Self.draw(code: code, scale: scale, quietModules: quietModules, in: cg)
        return UIGraphicsGetImageFromCurrentImageContext()
        #else
        var format = UIGraphicsImageRendererFormat.default()
        format.opaque = true
        return UIGraphicsImageRenderer(size: CGSize(width: pixelSide, height: pixelSide), format: format).image { renderer in
            Self.draw(code: code, scale: scale, quietModules: quietModules, in: renderer.cgContext)
        }
        #endif
    }

    private static func draw(code: QRCode, scale: Int, quietModules: Int, in cg: CGContext) {
        let modules = code.size
        let side = modules + quietModules * 2
        let pixelSide = CGFloat(side * scale)
        cg.setFillColor(UIColor.white.cgColor)
        cg.fill(CGRect(x: 0, y: 0, width: pixelSide, height: pixelSide))
        cg.setFillColor(UIColor.black.cgColor)
        for y in 0..<modules {
            for x in 0..<modules where code.getModule(x: x, y: y) {
                cg.fill(CGRect(
                    x: CGFloat(quietModules + x) * CGFloat(scale),
                    y: CGFloat(quietModules + y) * CGFloat(scale),
                    width: CGFloat(scale),
                    height: CGFloat(scale)
                ))
            }
        }
    }

    /// Maps the legacy correction-level string to the generator's ECC levels;
    /// anything unrecognized defaults to level L.
    private static func ecc(for correctionLevel: String) -> QRCodeECC {
        switch correctionLevel.uppercased() {
        case "M": return .medium
        case "Q": return .quartile
        case "H": return .high
        default: return .low
        }
    }
}