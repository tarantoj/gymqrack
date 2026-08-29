import CoreLocation
import SwiftUI

/// Renders the fresh entry QR. The payload is opaque and short-lived, so the
/// store re-fetches on a timer while this view is on screen; the "New code"
/// button forces an immediate refresh before scanning.
struct QRView: View {
    let club: Center
    let distance: CLLocationDistance?
    @EnvironmentObject private var store: SessionStore

    var body: some View {
        VStack(spacing: 4) {
            Text(club.clubName ?? "VivaGym")
                .font(.caption2)
                .lineLimit(1)
                .foregroundStyle(.secondary)

            if let payload = store.qrPayload, let image = QRImageRenderer.makeImage(payload: payload) {
                Image(uiImage: image)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .padding(4)
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
                    .task {
                        await store.refreshQR()
                    }
            }

            Text(statusText)
                .font(.caption2)
                .foregroundStyle(.secondary)

            Button("New code") {
                Task { await store.refreshQR() }
            }
            .font(.caption2)
        }
    }

    private var statusText: String {
        let updated = store.qrUpdated.map(Self.shortTime) ?? "Fetching…"
        if let distance {
            return "\(updated) · \(String(format: "%.0f", distance)) m"
        }
        return updated
    }

    private static func shortTime(_ date: Date) -> String {
        let formatter = DateFormatter()
        formatter.dateStyle = .none
        formatter.timeStyle = .short
        return formatter.string(from: date)
    }
}