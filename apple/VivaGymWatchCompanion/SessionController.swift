import Combine
import Foundation

/// Backs the companion login screen: runs the two-stage VivaGym login, stores
    /// the session, and pushes it to the watch via WatchConnectivity.
    final class SessionController: ObservableObject {
    @Published var isSignedIn: Bool
    @Published var email: String
    @Published var clubs: [Center] = []
    @Published var isWorking = false
    @Published var errorMessage: String?
    /// Why the club list failed to load (shown in the signed-in footer).
    @Published var clubsError: String?

    private let client = VivaGymClient()
    private let resolver = ClubPlaceResolver()

    init() {
        let session = KeychainSessionStore.load()
        isSignedIn = session != nil
        email = session?.email ?? ""
    }

    /// Fetches the club list on launch using the stored session (the club list
    /// is otherwise only loaded during an interactive sign-in). Refreshes the
    /// access token first if it is near expiry.
    @MainActor
    func refreshStoredSession() async {
        guard KeychainSessionStore.load() != nil else { return }
        do {
            let session = try await validSession()
            clubs = try await client.fetchUserClubs(accessToken: session.accessToken)
            clubsError = nil
            pushResolvedClubs()
        } catch {
            clubsError = message(for: error)
        }
    }

    /// Re-broadcasts the stored session to the watch. Called whenever the
    /// client's WCSession activates or the app returns to the foreground, so a
    /// watch that never saw a live login still gets the session.
    @MainActor
    func pushSessionToWatch() {
        guard let session = KeychainSessionStore.load() else { return }
        SessionSync.shared.sendSession(session)
    }

    @MainActor
    private func validSession() async throws -> Session {
        guard var session = KeychainSessionStore.load() else {
            throw VivaGymError(message: "No session", status: 401)
        }
        if session.isExpiredOrExpiring {
            let pair = try await client.refresh(refreshToken: session.refreshToken)
            session.accessToken = pair.accessToken
            session.refreshToken = pair.refreshToken ?? session.refreshToken
            session.expiresIn = pair.expiresIn ?? 600
            session.issuedAt = Date()
            try KeychainSessionStore.save(session)
        }
        return session
    }

    @MainActor
    func signIn(email: String, password: String) async {
        isWorking = true
        errorMessage = nil
        clubsError = nil
        defer { isWorking = false }

        do {
            let pair = try await client.login(email: email, password: password)
            let session = Session(
                email: email,
                accessToken: pair.accessToken,
                refreshToken: pair.refreshToken ?? "",
                issuedAt: Date(),
                expiresIn: pair.expiresIn ?? 600,
                locale: VivaGymConfig.locale
            )
            try KeychainSessionStore.save(session)
            self.email = email
            isSignedIn = true
            SessionSync.shared.sendSession(session)
            do {
                clubs = try await client.fetchUserClubs(accessToken: session.accessToken)
                pushResolvedClubs()
            } catch {
                clubsError = message(for: error)
            }
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    @MainActor
    func signOut() {
        KeychainSessionStore.clear()
        isSignedIn = false
        email = ""
        clubs = []
        errorMessage = nil
        clubsError = nil
        SessionSync.shared.sendSignedOut()
    }

    /// Resolves the current club list to Apple Maps places off the login
    /// critical path and pushes them to the watch.
    @MainActor
    private func pushResolvedClubs() {
        let current = clubs
        Task {
            let places = await resolver.resolveAll(current)
            guard !places.isEmpty else { return }
            SessionSync.shared.sendClubs(places)
        }
    }

    private func message(for error: Error) -> String {
        if let error = error as? VivaGymError {
            return error.status == 0
                ? error.message
                : "HTTP \(error.status): \(error.message)"
        }
        return error.localizedDescription
    }
}