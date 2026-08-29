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
                }
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active {
                        store.start()
                    }
                }
        }
    }
}