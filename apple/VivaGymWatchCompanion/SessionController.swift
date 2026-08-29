import Combine
import Foundation

/// Backs the companion login screen: runs the two-stage VivaGym login, stores
/// the session in the shared keychain (visible to the watch app), and nudges a
/// running watch app to reload via WatchConnectivity.
final class SessionController: ObservableObject {
    @Published var isSignedIn: Bool
    @Published var email: String
    @Published var clubs: [Center] = []
    @Published var isWorking = false
    @Published var errorMessage: String?

    private let client = VivaGymClient()

    init() {
        let session = KeychainSessionStore.load()
        isSignedIn = session != nil
        email = session?.email ?? ""
    }

    @MainActor
    func signIn(email: String, password: String) async {
        isWorking = true
        errorMessage = nil
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
            SessionSync.shared.notifySessionChanged()
            clubs = (try? await client.fetchUserClubs(accessToken: session.accessToken)) ?? []
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
        SessionSync.shared.notifySessionChanged()
    }
}