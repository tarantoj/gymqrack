import SwiftUI

@main
struct VivaGymWatchApp: App {
    @StateObject private var store = SessionStore()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            VivaGymWatchRootView()
                .environmentObject(store)
                .onAppear {
                    SessionSync.shared.onSessionChanged = { store.start() }
                    SessionSync.shared.activate()
                    store.start()
                    requestSessionIfNeeded()
                }
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active {
                        store.start()
                        requestSessionIfNeeded()
                    }
                }
        }
    }

    /// Pulls the session from a reachable phone shortly after activation, while
    /// the WCSession finishes activating. Queued pushes (application context /
    /// user info) cover the case where the phone isn't reachable right now.
    private func requestSessionIfNeeded() {
        Task {
            for _ in 0..<6 {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                if KeychainSessionStore.load() != nil { break }
                SessionSync.shared.requestSession()
            }
        }
    }
}