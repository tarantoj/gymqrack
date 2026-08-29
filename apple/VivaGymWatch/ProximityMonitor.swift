import CoreLocation
import Foundation
import UserNotifications

/// Owns the `CLLocationManager`: one-shot location for the proximity gate plus
/// `CLCircularRegion` geofences that surface an arrival notification and wake
/// the app into its QR view.
final class ProximityMonitor: NSObject, CLLocationManagerDelegate {
    private let manager = CLLocationManager()
    private var clubs: [Center] = []

    private(set) var location: CLLocation?
    private(set) var authorizationStatus: CLAuthorizationStatus = .notDetermined

    /// Called (on the main thread) after a location update, authorization change,
    /// or region entry so the store can recompute proximity.
    var onUpdate: (() -> Void)?
    /// Called when a monitored club geofence is entered.
    var onRegionEnter: ((Center) -> Void)?

    override init() {
        super.init()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyHundredMeters
    }

    func requestAuthorization() {
        manager.requestAlwaysAuthorization()
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
    }

    func startLocation() {
        if CLLocationManager.locationServicesEnabled() {
            manager.requestLocation()
        } else {
            onUpdate?()
        }
    }

    func registerClubs(_ newClubs: [Center]) {
        clubs = newClubs
        manager.monitoredRegions.forEach { manager.stopMonitoring(for: $0) }
        for club in newClubs.prefix(20) {
            guard let coordinate = club.coordinate else { continue }
            let region = CLCircularRegion(
                center: coordinate,
                radius: VivaGymConfig.regionRadius,
                identifier: "club-\(club.clubNo)"
            )
            region.notifyOnEntry = true
            region.notifyOnExit = false
            manager.startMonitoring(for: region)
        }
    }

    func distance(to club: Center) -> CLLocationDistance? {
        guard let location, let clubLocation = club.location else { return nil }
        return location.distance(from: clubLocation)
    }

    // MARK: - CLLocationManagerDelegate

    func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        hopToMain { [weak self] in
            guard let self else { return }
            self.authorizationStatus = CLLocationManager.authorizationStatus
            if self.authorizationStatus == .authorizedAlways || self.authorizationStatus == .authorizedWhenInUse {
                self.manager.requestLocation()
            }
            self.onUpdate?()
        }
    }

    func locationManager(_ manager: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
        let fix = locations.last
        hopToMain { [weak self] in
            self?.location = fix
            self?.onUpdate?()
        }
    }

    func locationManager(_ manager: CLLocationManager, didFailWithError error: Error) {
        hopToMain { [weak self] in
            self?.onUpdate?()
        }
    }

    func locationManager(_ manager: CLLocationManager, didEnterRegion region: CLRegion) {
        guard let club = club(for: region.identifier) else { return }
        postArrivalNotification(for: club)
        hopToMain { [weak self] in
            self?.onRegionEnter?(club)
        }
    }

    private func club(for identifier: String) -> Center? {
        let prefix = "club-"
        guard identifier.hasPrefix(prefix), let clubNo = Int(identifier.dropFirst(prefix.count)) else {
            return nil
        }
        return clubs.first { $0.clubNo == clubNo }
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