import CoreImage
import UIKit

/// Renders a VivaGym QR payload to a bitmap.
///
/// The payload is opaque server-signed text (`exerp:checkin:<memberRef>-<millis>-<digest>`)
/// that must be passed through unchanged. Rendering uses CoreImage's
/// `CIQRCodeGenerator` at error-correction level **L**, padding with a
/// 4-module quiet zone — the same parameters the VivaGym app's ZXing encoder and
/// gymqrack's `rsc.io/qr` use, so the scanner reads it identically.
public enum QRImageRenderer {
    public static func makeImage(
        payload: String,
        correctionLevel: String = "L",
        quietModules: Int = 4,
        minSidePixels: Int = 400
    ) -> UIImage? {
        guard !payload.isEmpty, let data = payload.data(using: .utf8) else { return nil }
        guard let filter = CIFilter(name: "CIQRCodeGenerator") else { return nil }
        filter.setValue(data, forKey: "inputMessage")
        filter.setValue(correctionLevel, forKey: "inputCorrectionLevel")
        guard let output = filter.outputImage else { return nil }

        // CIQRCodeGenerator emits one pixel per module.
        let modules = Int(output.extent.width.rounded())
        guard modules > 1 else { return nil }

        let side = modules + quietModules * 2
        let scale = max(2, Int(ceil(Double(minSidePixels) / Double(side))))
        let pixelSide = CGFloat(side * scale)
        // Nearest-neighbor upscale keeps module edges crisp; drawn 1:1 below.
        let scaled = output.samplingNearest()
            .transformed(by: CGAffineTransform(scaleX: CGFloat(scale), y: CGFloat(scale)))

        return UIGraphicsImageRenderer(size: CGSize(width: pixelSide, height: pixelSide)).image { ctx in
            UIColor.white.setFill()
            ctx.fill(CGRect(origin: .zero, size: CGSize(width: pixelSide, height: pixelSide)))
            let origin = CGFloat(quietModules * scale)
            UIImage(ciImage: scaled).draw(in: CGRect(
                x: origin, y: origin,
                width: pixelSide - origin * 2, height: pixelSide - origin * 2
            ))
        }
    }
}