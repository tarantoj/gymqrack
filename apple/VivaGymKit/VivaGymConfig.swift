import CoreLocation
import Foundation

/// App-level constants for the VivaGym API client.
///
/// The client id/secret are the public app credentials extracted from the
/// VivaGym app's `BuildConfig` (see `apk/jadx-out/sources/com/vitale/coredata/BuildConfig.java`
/// and `docs/api.md`). They are the same values the gymqrack server uses for the
/// anonymous `client_credentials` grant; they are not member secrets.
public enum VivaGymConfig {
    public static let baseURL = URL(string: "https://vivagym.myvitale.com")!
    public static let clientID = "4_43uq8rgou3y88ckkk0sgg8c408w4gwsssg8owg0ow4wcocgw0w"
    public static let clientSecret = "1uiljdab2misc4owsc0kg0cw0kgw0k0gkgk0k8k488w8sskk4s"
    /// Locale for the login endpoint (es | en | pt).
    public static let locale = "es"

    /// Distance (m) within which the entry QR is shown automatically.
    public static let nearThreshold: CLLocationDistance = 250
    /// Geofence radius (m) that triggers the "open to scan" notification.
    public static let regionRadius: CLLocationDistance = 200
    /// How often the QR payload is re-fetched while the QR screen is visible.
    public static let qrRefreshInterval: TimeInterval = 45
    /// Access-token safety margin (s) before expiry, matching gymqrack.
    public static let tokenSafetyMargin: TimeInterval = 10
}