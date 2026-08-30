import Foundation
import WatchConnectivity

/// WatchConnectivity bridge between the iPhone companion and the watch.
///
/// The shared-keychain access group is unreliable for standalone watch apps on
/// free teams, so the session is transferred explicitly: the companion pushes
/// the decoded `Session` (or a sign-out event) after login, and the watch
/// persists it in its own keychain and reloads. A watch with no session can
/// request one back from a reachable phone. WatchConnectivity transports are
/// end-to-end encrypted between the paired devices.
public final class SessionSync: NSObject, WCSessionDelegate {
    public static let shared = SessionSync()

    /// Invoked on the main thread when the session may have changed, so the
    /// watch store can reload from (local) storage.
    public var onSessionChanged: (() -> Void)?
    /// Invoked on the main thread once the session is `.activated`, e.g. for the
    /// companion to re-broadcast the stored session to a not-yet-synced watch.
    public var onActivated: (() -> Void)?

    public let session: WCSession

    /// Guards against relaying the same change twice (application context +
    /// live message arrive together), which would restart the watch store.
    private var lastRelayAt: Date?

    private static let eventKey = "event"
    private static let sessionKey = "session"
    private static let eventSession = "session"
    private static let eventSignedOut = "signedOut"
    private static let eventRequest = "requestSession"

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

    // MARK: - Companion → watch

    /// Called by the companion after saving/creating the session.
    public func sendSession(_ session: Session) {
        guard let data = try? JSONEncoder().encode(session) else { return }
        transmit(payload(event: Self.eventSession, data: data))
    }

    /// Called by the companion after sign-out so the watch clears its copy.
    public func sendSignedOut() {
        transmit(payload(event: Self.eventSignedOut, data: nil))
    }

    private func transmit(_ payload: [String: Any]) {
        guard WCSession.isSupported(), session.activationState == .activated else { return }
        // Application context and user-info transfers are queued and delivered
        // whenever the watch app is (next) activated, so they work even if the
        // watch isn't running right now; sendMessage covers the live case.
        do {
            try session.updateApplicationContext(payload)
        } catch {}
        session.transferUserInfo(payload)
        if session.isReachable {
            session.sendMessage(payload, replyHandler: nil, errorHandler: nil)
        }
    }

    // MARK: - Watch → companion

    /// Asks a reachable phone for the current session (used at watch launch
    /// when nothing is stored yet, e.g. the phone paired after login).
    public func requestSession() {
        guard WCSession.isSupported(), session.activationState == .activated, session.isReachable else { return }
        session.sendMessage([Self.eventKey: Self.eventRequest], replyHandler: { [weak self] reply in
            if let data = reply[Self.sessionKey] as? Data, let stored = try? JSONDecoder().decode(Session.self, from: data) {
                try? KeychainSessionStore.save(stored)
            } else {
                KeychainSessionStore.clear()
            }
            self?.relayChange()
        }, errorHandler: nil)
    }

    // MARK: - Incoming payloads

    private func handle(payload: [String: Any]) {
        switch payload[Self.eventKey] as? String {
        case Self.eventSession:
            if let data = payload[Self.sessionKey] as? Data, let stored = try? JSONDecoder().decode(Session.self, from: data) {
                try? KeychainSessionStore.save(stored)
            }
            relayChange()
        case Self.eventSignedOut:
            KeychainSessionStore.clear()
            relayChange()
        default:
            break
        }
    }

    private func payload(event: String, data: Data?) -> [String: Any] {
        var payload: [String: Any] = [Self.eventKey: event]
        if let data {
            payload[Self.sessionKey] = data
        }
        return payload
    }

    // MARK: - WCSessionDelegate

    private func relayChange() {
        let now = Date()
        if let lastRelayAt, now.timeIntervalSince(lastRelayAt) < 1 { return }
        lastRelayAt = now
        DispatchQueue.main.async { [weak self] in
            self?.onSessionChanged?()
        }
    }

    public func session(_ session: WCSession, activationDidCompleteWith activationState: WCSessionActivationState, error: Error?) {
        if activationState == .activated {
            DispatchQueue.main.async { [weak self] in
                self?.onActivated?()
            }
        }
    }

    public func session(_ session: WCSession, didReceiveMessage message: [String: Any]) {
        handle(payload: message)
    }

    public func session(_ session: WCSession, didReceiveMessage message: [String: Any], replyHandler: @escaping ([String: Any]) -> Void) {
        if message[Self.eventKey] as? String == Self.eventRequest {
            var reply = payload(event: Self.eventSignedOut, data: nil)
            if let stored = KeychainSessionStore.load(), let data = try? JSONEncoder().encode(stored) {
                reply = payload(event: Self.eventSession, data: data)
            }
            replyHandler(reply)
        } else {
            handle(payload: message)
            replyHandler([:])
        }
    }

    public func session(_ session: WCSession, didReceiveApplicationContext applicationContext: [String: Any]) {
        handle(payload: applicationContext)
    }

    public func session(_ session: WCSession, didReceiveUserInfo userInfo: [String: Any]) {
        handle(payload: userInfo)
    }

    #if os(iOS)
    public func sessionDidBecomeInactive(_ session: WCSession) {}
    public func sessionDidDeactivate(_ session: WCSession) {
        // Re-activate on iOS so the app can keep exchanging messages.
        session.activate()
    }
    #endif
}