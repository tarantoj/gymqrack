import Vision
import XCTest

final class QRImageRendererTests: XCTestCase {
    private let payload =
        "exerp:checkin:123p45678-1786821607534-0123456789abcdef0123456789abcdef"

    func testRendersRealisticPayload() {
        let image = QRImageRenderer.makeImage(payload: payload)
        XCTAssertNotNil(image)
    }

    func testNilForEmptyPayload() {
        XCTAssertNil(QRImageRenderer.makeImage(payload: ""))
    }

    func testSamePayloadRendersSameSize() {
        let a = QRImageRenderer.makeImage(payload: payload)
        let b = QRImageRenderer.makeImage(payload: payload)
        XCTAssertEqual(a?.size, b?.size)
        XCTAssertGreaterThan(a?.size.width ?? 0, 0)
    }

    func testMinSideRespected() {
        let image = QRImageRenderer.makeImage(payload: payload, minSidePixels: 512)
        XCTAssertGreaterThanOrEqual(image?.size.width ?? 0, 512)
    }

    func testDecodesPayloadFromRenderedImage() throws {
        let image = try XCTUnwrap(QRImageRenderer.makeImage(payload: payload))
        let cgImage = try XCTUnwrap(image.cgImage)
        let request = VNDetectBarcodesRequest()
        request.symbologies = [.qr]
        try VNImageRequestHandler(cgImage: cgImage).perform([request])
        let results = request.results ?? []
        XCTAssertEqual(results.count, 1, "expected exactly one QR in the image")
        XCTAssertEqual(results.first?.payloadStringValue, payload)
    }
}