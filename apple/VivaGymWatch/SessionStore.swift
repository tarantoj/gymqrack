import Combine
import CoreLocation
import Foundation

/// Drives the watch UI. Resolves the session from the shared keychain, keeps it
/// fresh, fetches the member's clubs, and decides — from the current location —
/// whether to show the entry QR, a distance hint, or a sign-in prompt. While the
/// QR is visible it is re-fetched on a timer because the payload is opaque and
/// short-lived.
final class SessionStore: ObservableObject {
    enum State: Equatable {
        case signedOut
        case preparing
        case locating
        case near(Center, CLLocationDistance?)
        case far(Center, CLLocationDistance)
        case failed(String)
    }

    @Published var state: State = .preparing
    @Published var qrPayload: String?
    @Published var qrUpdated: Date?

    private let client: VivaGymClient
    private let proximity: ProximityMonitor
    private var clubs: [Center] = []
    private var forceQR = false
    private var refreshingToken = false
    private var refreshTask: Task<Void, Never>?

    init(client: VivaGymClient = VivaGymClient(), proximity: ProximityMonitor = ProximityMonitor()) {
        self.client = client
        self.proximity = proximity
        proximity.onUpdate = { [weak self] in
            Task { @MainActor in self?.recompute() }
        }
        proximity.onRegionEnter = { [weak self] club in
            Task { @MainActor in self?.regionEntered(club) }
        }
    }

    // MARK: - Lifecycle

    @MainActor
    func start() {
        stopQRRefresh()
        state = .preparing
        forceQR = false
        guard KeychainSessionStore.load() != nil else {
            state = .signedOut
            return
        }
        Task { await loadData() }
    }

    // MARK: - Data loading

    @MainActor
    private func loadData() async {
        do {
            let session = try await ensureValidSession()
            state = .locating
            proximity.requestAuthorization()
            proximity.startLocation()
            clubs = try await client.fetchUserClubs(accessToken: session.accessToken)
            proximity.registerClubs(clubs)
            recompute()
        } catch let error as VivaGymError {
            if error.status == 400 || error.status == 401 || error.status == 403 {
                KeychainSessionStore.clear()
                state = .signedOut
            } else {
                state = .failed(error.localizedDescription)
            }
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    // MARK: - Session

    @MainActor
    private func ensureValidSession() async throws -> Session {
        guard var session = KeychainSessionStore.load() else {
            throw VivaGymError(message: "No session", status: 401)
        }
        if session.isExpiredOrExpiring {
            guard !refreshingToken else {
                throw VivaGymError(message: "Refreshing token…", status: 0)
            }
            refreshingToken = true
            defer { refreshingToken = false }
            do {
                let pair = try await client.refresh(refreshToken: session.refreshToken)
                session.accessToken = pair.accessToken
                session.refreshToken = pair.refreshToken ?? session.refreshToken
                session.expiresIn = pair.expiresIn ?? 600
                session.issuedAt = Date()
                try KeychainSessionStore.save(session)
            } catch {
                KeychainSessionStore.clear()
                throw error
            }
        }
        return session
    }

    // MARK: - Proximity

    @MainActor
    private func regionEntered(_ club: Center) {
        forceQR = true
        // Refresh immediately: the payload must be fresh when scanning.
        Task { await refreshQR() }
        recompute()
    }

    @MainActor
    func forceShowQR() {
        forceQR = true
        recompute()
    }

    @MainActor
    private func recompute() {
        let current = proximity.location
        guard !clubs.isEmpty else {
            if forceQR { state = .failed("No club list available") }
            return
        }

        let sorted = clubs
            .map { (club: $0, distance: Self.distance(from: current, to: $0)) }
            .sorted { $0.distance < $1.distance }
        guard let nearest = sorted.first else {
            state = .failed("No club locations found")
            return
        }
        let finiteDistance: CLLocationDistance? = nearest.distance.isFinite ? nearest.distance : nil

        if forceQR {
            state = .near(nearest.club, finiteDistance)
            startQRRefresh()
            return
        }

        guard let current, let distance = proximity.distance(to: nearest.club) else {
            state = .locating
            return
        }

        if distance <= VivaGymConfig.nearThreshold {
            state = .near(nearest.club, distance)
            startQRRefresh()
        } else {
            stopQRRefresh()
            state = .far(nearest.club, distance)
        }
    }

    private static func distance(from location: CLLocation?, to club: Center) -> CLLocationDistance {
        guard let location, let clubLocation = club.location else { return .greatestFiniteMagnitude }
        return location.distance(from: clubLocation)
    }

    // MARK: - QR

    @MainActor
    func refreshQR() async {
        guard case .near = state else { return }
        do {
            let session = try await ensureValidSession()
            let payload = try await client.fetchQR(accessToken: session.accessToken)
            qrPayload = payload
            qrUpdated = Date()
        } catch {}
    }

    @MainActor
    private func startQRRefresh() {
        stopQRRefresh()
        Task { await refreshQR() }
        refreshTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: UInt64(VivaGymConfig.qrRefreshInterval * 1_000_000_000))
                guard !Task.isCancelled else { break }
                await refreshQR()
            }
        }
    }

    @MainActor
    private func stopQRRefresh() {
        refreshTask?.cancel()
        refreshTask = nil
    }
}