import CoreLocation
import Foundation

/// A VivaGym club (one of the member's clubs).
///
/// Mirrors the `Center` model returned by `GET api/v2.0/exerp/user-clubs`
/// (`apk/jadx-out/sources/com/myvitale/api/domain/model/Center.java`). Coding
/// tolerates `latitude`/`longitude` arriving as either strings or numbers, and
/// `clubNo` as a number or string.
public struct Center: Codable, Hashable, Identifiable, Sendable {
    public let clubNo: Int
    public let clubName: String?
    public let latitude: Double?
    public let longitude: Double?
    public let address1: String?
    public let address2: String?
    public let address3: String?
    public let postalCode: String?
    public let urlName: String?
    public let title: String?
    public let description: String?
    public let telephone: String?
    public let regionDesc: String?
    public let autonomousRegion: String?
    public let isOpen: Int?

    public var id: Int { clubNo }

    public var coordinate: CLLocationCoordinate2D? {
        guard let latitude, let longitude else { return nil }
        return CLLocationCoordinate2D(latitude: latitude, longitude: longitude)
    }

    public var location: CLLocation? {
        coordinate.map { CLLocation(latitude: $0.latitude, longitude: $0.longitude) }
    }

    public init(
        clubNo: Int,
        clubName: String? = nil,
        latitude: Double? = nil,
        longitude: Double? = nil,
        address1: String? = nil,
        address2: String? = nil,
        address3: String? = nil,
        postalCode: String? = nil,
        urlName: String? = nil,
        title: String? = nil,
        description: String? = nil,
        telephone: String? = nil,
        regionDesc: String? = nil,
        autonomousRegion: String? = nil,
        isOpen: Int? = nil
    ) {
        self.clubNo = clubNo
        self.clubName = clubName
        self.latitude = latitude
        self.longitude = longitude
        self.address1 = address1
        self.address2 = address2
        self.address3 = address3
        self.postalCode = postalCode
        self.urlName = urlName
        self.title = title
        self.description = description
        self.telephone = telephone
        self.regionDesc = regionDesc
        self.autonomousRegion = autonomousRegion
        self.isOpen = isOpen
    }

    private enum CodingKeys: String, CodingKey {
        case clubNo, clubName, latitude, longitude
        case address1, address2, address3, postalCode
        case urlName, title, description, telephone
        case regionDesc, autonomousRegion, isOpen
    }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        clubNo = Self.decodeInt(c, for: .clubNo) ?? 0
        clubName = try c.decodeIfPresent(String.self, forKey: .clubName)
        latitude = Self.decodeDouble(c, for: .latitude)
        longitude = Self.decodeDouble(c, for: .longitude)
        address1 = try c.decodeIfPresent(String.self, forKey: .address1)
        address2 = try c.decodeIfPresent(String.self, forKey: .address2)
        address3 = try c.decodeIfPresent(String.self, forKey: .address3)
        postalCode = try c.decodeIfPresent(String.self, forKey: .postalCode)
        urlName = try c.decodeIfPresent(String.self, forKey: .urlName)
        title = try c.decodeIfPresent(String.self, forKey: .title)
        description = try c.decodeIfPresent(String.self, forKey: .description)
        telephone = try c.decodeIfPresent(String.self, forKey: .telephone)
        regionDesc = try c.decodeIfPresent(String.self, forKey: .regionDesc)
        autonomousRegion = try c.decodeIfPresent(String.self, forKey: .autonomousRegion)
        isOpen = Self.decodeInt(c, for: .isOpen)
    }

    private static func decodeInt(_ c: KeyedDecodingContainer<CodingKeys>, for key: CodingKeys) -> Int? {
        if let v = try? c.decodeIfPresent(Int.self, forKey: key) { return v }
        if let s = try? c.decodeIfPresent(String.self, forKey: key) { return Int(s) }
        return nil
    }

    private static func decodeDouble(_ c: KeyedDecodingContainer<CodingKeys>, for key: CodingKeys) -> Double? {
        if let v = try? c.decodeIfPresent(Double.self, forKey: key) { return v }
        if let s = try? c.decodeIfPresent(String.self, forKey: key) {
            let trimmed = s.trimmingCharacters(in: .whitespaces)
            return trimmed.isEmpty ? nil : Double(s.replacingOccurrences(of: ",", with: "."))
        }
        return nil
    }
}

/// The member's VivaGym session persisted in the shared keychain.
public struct Session: Codable, Equatable, Sendable {
    public var email: String
    public var accessToken: String
    public var refreshToken: String
    public var issuedAt: Date
    /// Access-token lifetime in seconds.
    public var expiresIn: Int
    public var locale: String

    public init(
        email: String,
        accessToken: String,
        refreshToken: String,
        issuedAt: Date = Date(),
        expiresIn: Int = 600,
        locale: String = VivaGymConfig.locale
    ) {
        self.email = email
        self.accessToken = accessToken
        self.refreshToken = refreshToken
        self.issuedAt = issuedAt
        self.expiresIn = expiresIn
        self.locale = locale
    }

    /// True when the access token is expired (or within the safety margin),
    /// so callers should refresh before the next upstream call.
    public var isExpiredOrExpiring: Bool {
        let elapsed = Date().timeIntervalSince(issuedAt)
        return elapsed >= Double(expiresIn) - VivaGymConfig.tokenSafetyMargin
    }
}

/// A club together with its distance from the current location.
public struct ClubDistance: Sendable {
    public let club: Center
    public let distance: CLLocationDistance
}