import CoreLocation
import SwiftUI

/// Root view switching on the store's state machine.
struct VivaGymWatchRootView: View {
    @EnvironmentObject private var store: SessionStore

    var body: some View {
        Group {
            switch store.state {
            case .signedOut:
                SignedOutView()
            case .preparing, .locating:
                PreparingView()
            case .near(let club, let distance):
                QRView(club: club, distance: distance)
            case .far(let club, let distance):
                DistanceView(club: club, distance: distance)
            case .failed(let message):
                ErrorView(message: message)
            }
        }
        .animation(.default, value: store.state)
    }
}

/// Shown when the shared keychain has no session yet.
struct SignedOutView: View {
    var body: some View {
        VStack(spacing: 8) {
            Image(systemName: "iphone")
                .font(.title)
            Text("Sign in on your iPhone")
                .font(.headline)
                .multilineTextAlignment(.center)
            Text("Open the VivaGym Watch app on your iPhone once, sign in with your VivaGym details, and the QR will appear here automatically.")
                .font(.caption2)
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
        }
        .padding()
    }
}

/// Shown while the session/clubs/location are being fetched.
struct PreparingView: View {
    @EnvironmentObject private var store: SessionStore

    var body: some View {
        VStack(spacing: 10) {
            ProgressView()
            Text("Getting ready…")
                .font(.caption)
                .foregroundStyle(.secondary)
            if case .locating = store.state {
                Button("Show QR anyway") {
                    store.forceShowQR()
                }
                .font(.caption2)
            }
        }
        .padding()
    }
}

/// Shown when not (yet) within the proximity threshold of a club.
struct DistanceView: View {
    let club: Center
    let distance: CLLocationDistance
    @EnvironmentObject private var store: SessionStore

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: "figure.run")
                .font(.title)
            Text(club.clubName ?? "VivaGym")
                .font(.headline)
                .multilineTextAlignment(.center)
            Text(String(format: "%.1f km away", distance / 1000))
                .font(.caption)
                .foregroundStyle(.secondary)
            Text("Your entry QR unlocks when you arrive.")
                .font(.caption2)
                .multilineTextAlignment(.center)
                .foregroundStyle(.secondary)
            Button("Show QR anyway") {
                store.forceShowQR()
            }
            .font(.caption2)
        }
        .padding()
    }
}

/// Shown when something went wrong (network, upstream, …).
struct ErrorView: View {
    let message: String
    @EnvironmentObject private var store: SessionStore

    var body: some View {
        VStack(spacing: 10) {
            Image(systemName: "exclamationmark.triangle")
                .font(.title)
                .foregroundStyle(.orange)
            Text(message)
                .font(.caption2)
                .multilineTextAlignment(.center)
            Button("Retry") {
                store.start()
            }
            .font(.caption2)
        }
        .padding()
    }
}