import Foundation
import WatchConnectivity

/// Minimal WatchConnectivity bridge so the iPhone companion can nudge a running
/// watch app (or a waiting watch app) to reload the session from the shared
/// keychain right after login/sign-out.
public final class SessionSync: NSObject, WCSessionDelegate {
    public static let shared = SessionSync()

    /// Invoked on the main thread when the companion reports the session changed.
    public var onSessionChanged: (() -> Void)?

    public let session: WCSession

    public override convenience init() {
        self.init(session: WCSession.default)
    }

    public init(session: WCSession) {
        self.session = session
        super.init()
    }

    public func activate() {
        guard WCSession.isSupported() else { return }
        session.delegate = self
        session.activate()
    }

    /// Called by the companion after saving/clearing the session.
    public func notifySessionChanged() {
        guard WCSession.isSupported(), session.activationState == .activated else { return }
        let payload: [String: Any] = ["event": "sessionChanged"]
        do {
            try session.updateApplicationContext(payload)
        } catch {}
        if session.isReachable {
            session.sendMessage(payload, replyHandler: nil, errorHandler: nil)
        }
    }

    // MARK: - WCSessionDelegate

    private func relayChange() {
        DispatchQueue.main.async { [weak self] in
            self?.onSessionChanged?()
        }
    }

    public func session(_ session: WCSession, activationDidCompleteWith activationState: WCSessionActivationState, error: Error?) {}

    public func session(_ session: WCSession, didReceiveMessage message: [String: Any]) {
        if message["event"] as? String == "sessionChanged" { relayChange() }
    }

    public func session(_ session: WCSession, didReceiveApplicationContext applicationContext: [String: Any]) {
        if applicationContext["event"] as? String == "sessionChanged" { relayChange() }
    }

    #if os(iOS)
    public func sessionDidBecomeInactive(_ session: WCSession) {}
    public func sessionDidDeactivate(_ session: WCSession) {
        // Re-activate on iOS so the app can keep exchanging messages.
        session.activate()
    }
    #endif
}