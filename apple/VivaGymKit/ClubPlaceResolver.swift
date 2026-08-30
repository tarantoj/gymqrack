import CoreLocation
import Foundation
import os

/// A club's resolved Apple Maps place, used to anchor the geofence to the real
/// venue instead of the (sometimes approximate) API coordinate.
///
/// Codable so resolved places can cross WatchConnectivity and be persisted on
/// the watch for warm launches without the phone.
public struct ClubPlace: Codable, Sendable, Equatable {
    public let clubNo: Int
    public let name: String?
    public let latitude: Double
    public let longitude: Double
    /// Distance from the original API coordinate, i.e. how much the anchor moved.
    public let apiDistance: CLLocationDistance?

    public init(
        clubNo: Int,
        name: String?,
        latitude: Double,
        longitude: Double,
        apiDistance: CLLocationDistance?
    ) {
        self.clubNo = clubNo
        self.name = name
        self.latitude = latitude
        self.longitude = longitude
        self.apiDistance = apiDistance
    }

    public var coordinate: CLLocationCoordinate2D {
        CLLocationCoordinate2D(latitude: latitude, longitude: longitude)
    }

    public var location: CLLocation {
        CLLocation(latitude: latitude, longitude: longitude)
    }
}

/// Forward-geocoding seam so `ClubPlaceResolver` is testable without hitting
/// Apple's servers. `CLGeocoder` already matches this signature asynchronously.
public protocol ClubGeocoding: Sendable {
    func geocodeAddressString(
        _ addressString: String,
        in region: CLRegion?,
        preferredLocale locale: Locale?
    ) async throws -> [CLPlacemark]
}

/// Default adapter that delegates to `CLGeocoder`. A fresh geocoder per call:
/// `CLGeocoder` forbids issuing a second request while one is in flight.
public struct CLGeocoderAdapter: ClubGeocoding {
    public init() {}

    public func geocodeAddressString(
        _ addressString: String,
        in region: CLRegion?,
        preferredLocale locale: Locale?
    ) async throws -> [CLPlacemark] {
        let geocoder = CLGeocoder()
        return try await geocoder.geocodeAddressString(addressString, in: region, preferredLocale: locale)
    }
}

/// Resolves each member club to its Apple Maps place so the proximity/geofence
/// anchor uses Apple's authoritative coordinate for the venue. Runs on the
/// iPhone companion; the watch keeps its own API coordinates as a fallback
/// until the resolved places are pushed over WatchConnectivity.
public struct ClubPlaceResolver: Sendable {
    private static let logger = Logger(subsystem: "com.vivagym.VivaGymKit", category: "club-place")

    private let geocoder: any ClubGeocoding
    private let regionRadius: CLLocationDistance
    private let mismatchThreshold: CLLocationDistance
    private let locale: Locale

    public init(
        geocoder: any ClubGeocoding = CLGeocoderAdapter(),
        regionRadius: CLLocationDistance = VivaGymConfig.clubPlaceRegionRadius,
        mismatchThreshold: CLLocationDistance = VivaGymConfig.clubPlaceMismatchThreshold,
        locale: Locale = Locale(identifier: VivaGymConfig.locale)
    ) {
        self.geocoder = geocoder
        self.regionRadius = regionRadius
        self.mismatchThreshold = mismatchThreshold
        self.locale = locale
    }

    /// Resolves a single club. Returns `nil` when there is no API coordinate to
    /// anchor on or geocoding fails, in which case the caller keeps the API
    /// coordinate.
    public func resolve(_ club: Center) async -> ClubPlace? {
        guard let api = club.location else { return nil }
        let region = CLCircularRegion(
            center: api.coordinate,
            radius: regionRadius,
            identifier: "club-\(club.clubNo)"
        )
        let placemarks = try? await geocoder.geocodeAddressString(
            Self.query(for: club),
            in: region,
            preferredLocale: locale
        )
        guard let placemark = placemarks?.first, let resolved = placemark.location else {
            return nil
        }
        let apiDistance = resolved.distance(from: api)
        if apiDistance > mismatchThreshold {
            Self.logger.warning(
                "Club \(club.clubNo) API coordinate is \(apiDistance, format: .fixed(precision: 0)) m from its Apple Maps place"
            )
        }
        return ClubPlace(
            clubNo: club.clubNo,
            name: placemark.name ?? club.clubName,
            latitude: resolved.coordinate.latitude,
            longitude: resolved.coordinate.longitude,
            apiDistance: apiDistance
        )
    }

    /// Resolves all clubs, in parallel, returning only the successes.
    public func resolveAll(_ clubs: [Center]) async -> [ClubPlace] {
        await withTaskGroup(of: ClubPlace?.self) { group in
            for club in clubs {
                group.addTask { await self.resolve(club) }
            }
            var places: [ClubPlace] = []
            for await place in group {
                if let place { places.append(place) }
            }
            return places
        }
    }

    /// Builds a geocodable query from the club's structured address, falling
    /// back to the club name alone when no address fields are present.
    static func query(for club: Center) -> String {
        let parts = [
            club.clubName,
            club.address1,
            club.address2,
            club.postalCode,
            club.regionDesc ?? club.autonomousRegion,
        ]
        let joined = parts.compactMap { $0?.trimmingCharacters(in: CharacterSet.whitespaces) }
            .filter { !$0.isEmpty }
            .joined(separator: ", ")
        return joined.isEmpty ? (club.clubName ?? "VivaGym") : joined
    }
}