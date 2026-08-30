import SwiftUI

/// The full-screen QR page: only the rendered QR image, nothing else.
///
/// The payload is opaque and short-lived, so the store re-fetches it on a timer
/// while near a club (and again when this page first appears). Rendering uses
/// `QRImageRenderer`, which draws the QR into an opaque bitmap (error-correction
/// level L + 4-module quiet zone, matching the VivaGym app's ZXing encoder and
/// gymqrack's rsc.io/qr).
struct QRPageView: View {
    @EnvironmentObject private var store: SessionStore

    var body: some View {
        Group {
            if let payload = store.qrPayload,
               let image = QRImageRenderer.makeImage(payload: payload) {
                Image(uiImage: image)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .ignoresSafeArea()
            } else if let qrError = store.qrError {
                VStack(spacing: 6) {
                    Image(systemName: "exclamationmark.triangle")
                        .font(.title3)
                        .foregroundStyle(.orange)
                    Text(qrError)
                        .font(.caption2)
                        .multilineTextAlignment(.center)
                        .foregroundStyle(.secondary)
                    Button("Retry") {
                        Task { await store.refreshQR() }
                    }
                    .font(.caption2)
                }
                .padding(.horizontal, 12)
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .task {
                        await store.refreshQR()
                    }
            }
        }
        .task {
            // Keep the code fresh (e.g. when arriving while looking at the
            // page); the 45 s timer covers the near-a-club case.
            if store.qrPayload != nil {
                await store.refreshQR()
            }
        }
    }
}