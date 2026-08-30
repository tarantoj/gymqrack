import CoreLocation
import Foundation
import UserNotifications

/// Owns location access for the watch.
///
/// The watchOS 26 SDK removed one-shot `requestLocation()`, region monitoring
/// (`CLCircularRegion` + `CLMonitor` are unavailable on watchOS) and the
/// delegate-based authorization prompts, so this streams fixes with
/// `CLLocationUpdate.liveUpdates()` (watchOS 10+); the updater drives
/// authorization on demand. Arrival is detected in-code from the fix stream — an
/// "open to scan" notification plus `onRegionEnter` fires once per club visit and
/// re-arms when the wearer leaves the geofence radius.
final class ProximityMonitor {
    private var updateTask: Task<Void, Never>?
    private var serviceSession: Any?
    private var clubs: [Center] = []
    private var lastEnteredClubNo: Int?

    /// Most recent location fix.
    private(set) var location: CLLocation?

    /// Called (on the main thread) after a location update so the store can
    /// recompute proximity.
    var onUpdate: (() -> Void)?
    /// Called when the wearer moves within a club's geofence radius.
    var onRegionEnter: ((Center) -> Void)?

    /// Requests notification permissions, surfaces the location authorization
    /// prompt, and starts the live location stream.
    func startLocation() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
        // The watchOS 26 SDK has no request*Authorization() calls on
        // CLLocationManager; a CLServiceSession requirement triggers the prompt
        // (watchOS 11+; this app targets watchOS 10, so guard the type usage).
        if #available(watchOS 11.0, *) {
            serviceSession = CLServiceSession(authorization: .whenInUse)
        }
        guard updateTask == nil else { return }
        updateTask = Task { [weak self] in
            await self?.runUpdates()
        }
    }

    /// Streams live locations, restarting after errors (e.g. before the user
    /// has answered the authorization prompt).
    private func runUpdates() async {
        do {
            for try await update in try CLLocationUpdate.liveUpdates() {
                self.handle(update: update)
            }
        } catch {
            try? await Task.sleep(nanoseconds: 3_000_000_000)
            guard !Task.isCancelled else { return }
            await runUpdates()
        }
    }

    func stopLocation() {
        updateTask?.cancel()
        updateTask = nil
        serviceSession = nil
    }

    func registerClubs(_ newClubs: [Center]) {
        clubs = newClubs
    }

    func distance(to club: Center) -> CLLocationDistance? {
        guard let location, let clubLocation = club.location else { return nil }
        return location.distance(from: clubLocation)
    }

    // MARK: - Arrival detection

    private func handle(update: CLLocationUpdate) {
        let fix = update.location
        hopToMain { [weak self] in
            guard let self else { return }
            self.location = fix
            self.detectArrival()
            self.onUpdate?()
        }
    }

    private func detectArrival() {
        guard let location else { return }
        var nearest: (club: Center, distance: CLLocationDistance)?
        for club in clubs {
            guard let clubLocation = club.location else { continue }
            let distance = location.distance(from: clubLocation)
            if nearest == nil || distance < nearest!.distance {
                nearest = (club, distance)
            }
        }
        guard let nearest else { return }
        if nearest.distance <= VivaGymConfig.regionRadius {
            if lastEnteredClubNo != nearest.club.clubNo {
                lastEnteredClubNo = nearest.club.clubNo
                postArrivalNotification(for: nearest.club)
                onRegionEnter?(nearest.club)
            }
        } else {
            lastEnteredClubNo = nil
        }
    }

    private func postArrivalNotification(for club: Center) {
        let content = UNMutableNotificationContent()
        content.title = "VivaGym \(club.clubName ?? "")"
        content.body = "You're at the gym — open your entry QR to scan in."
        content.sound = .default
        let request = UNNotificationRequest(
            identifier: "vivagym-arrival-\(club.clubNo)",
            content: content,
            trigger: nil
        )
        UNUserNotificationCenter.current().add(request)
    }

    private func hopToMain(_ block: @escaping () -> Void) {
        if Thread.isMainThread {
            block()
        } else {
            DispatchQueue.main.async(execute: block)
        }
    }
}