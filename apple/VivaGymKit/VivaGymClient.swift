import Foundation

/// Error carrying the HTTP status of a failed VivaGym call.
public struct VivaGymError: Error, LocalizedError, Sendable {
    public let message: String
    public let status: Int

    public init(message: String, status: Int) {
        self.message = message
        self.status = status
    }

    public var errorDescription: String? { message }
}

/// Result of a token-bearing call.
public struct TokenPair: Codable, Sendable, Equatable {
    public let accessToken: String
    public let refreshToken: String?
    public let expiresIn: Int?

    enum CodingKeys: String, CodingKey {
        case accessToken = "access_token"
        case refreshToken = "refresh_token"
        case expiresIn = "expires_in"
    }
}

/// A club entry from the public catalog (`{locale}/clubs`), which is the only
/// place the API currently returns coordinates (`gpsLocation` as "lat,lng").
public struct ClubLocation: Codable, Sendable, Equatable {
    public let id: Int?
    public let name: String?
    public let address: String?
    public let gpsLocation: String?
    public let imageUrl: String?
}

/// Direct client for the VivaGym API, mirroring `internal/vivagym/client.go`.
///
/// Auth flow (see `docs/api.md`):
/// 1. anonymous `client_credentials` grant -> "temp" token
/// 2. `exerp/newAuth` with email + password -> member token pair
/// 3. `api/email/refresh` to rotate a near-expiry access token
public struct VivaGymClient {
    public let baseURL: URL
    public let clientID: String
    public let clientSecret: String
    public let locale: String
    public let session: URLSession

    public init(
        baseURL: URL = VivaGymConfig.baseURL,
        clientID: String = VivaGymConfig.clientID,
        clientSecret: String = VivaGymConfig.clientSecret,
        locale: String = VivaGymConfig.locale,
        session: URLSession = .shared
    ) {
        self.baseURL = baseURL
        self.clientID = clientID
        self.clientSecret = clientSecret
        self.locale = locale
        self.session = session
    }

    // MARK: - Public API

    /// Two-stage member login: client_credentials -> exerp/newAuth.
    public func login(email: String, password: String) async throws -> TokenPair {
        let tempToken = try await clientCredentials()
        var comps = URLComponents()
        comps.queryItems = [
            URLQueryItem(name: "access_token", value: tempToken),
            URLQueryItem(name: "email", value: email),
            URLQueryItem(name: "password", value: password),
            URLQueryItem(name: "appName", value: "vivagym"),
        ]
        let body = comps.percentEncodedQuery.map { Data($0.utf8) }
        let data = try await httpData(
            url: url(for: "/api/v2.0/\(locale)/exerp/newAuth"),
            method: "POST",
            headers: ["Content-Type": "application/x-www-form-urlencoded"],
            body: body
        )
        return try Self.parseTokenResponse(data, what: "login")
    }

    /// Renews the access token using the refresh token. If the response omits a
    /// refresh token, the caller keeps the old one (matches client.go).
    public func refresh(refreshToken: String) async throws -> TokenPair {
        var comps = URLComponents()
        comps.queryItems = [URLQueryItem(name: "refresh_token", value: refreshToken)]
        let endpoint = "/api/email/refresh" + (comps.percentEncodedQuery.map { "?" + $0 } ?? "")
        let data = try await httpData(url: url(for: endpoint), method: "GET")
        var pair = try Self.parseTokenResponse(data, what: "refresh")
        if pair.refreshToken == nil || pair.refreshToken?.isEmpty == true {
            pair = TokenPair(accessToken: pair.accessToken, refreshToken: refreshToken, expiresIn: pair.expiresIn)
        }
        return pair
    }

    /// Returns the gym-entry QR payload as an opaque string. VivaGym returns
    /// the payload as a JSON-encoded string; fall back to the raw body if not.
    public func fetchQR(accessToken: String) async throws -> String {
        let data = try await httpData(
            url: url(for: "/api/v2.0/exerp/qr"),
            method: "GET",
            headers: ["Authorization": "Bearer \(accessToken)"]
        )
        if let payload = try? JSONDecoder().decode(String.self, from: data) {
            return payload
        }
        return String(decoding: data, as: UTF8.self)
    }

    /// The member's clubs with coordinates (`Center` fields clubNo, clubName,
    /// latitude/longitude). The `user-clubs` endpoint returns clubs without
    /// coordinates, and catalog `id`s use a different id space from Exerp
    /// `clubNo`s, so coordinates are joined from the public club catalog
    /// (`{locale}/clubs`, which has `gpsLocation`) by **name**.
    public func fetchUserClubs(accessToken: String) async throws -> [Center] {
        let data = try await httpData(
            url: url(for: "/api/v2.0/exerp/user-clubs"),
            method: "GET",
            headers: ["Authorization": "Bearer \(accessToken)"]
        )
        let clubs = try JSONDecoder().decode([Center].self, from: data)
        return try await attachCoordinates(clubs, accessToken: accessToken)
    }

    private func attachCoordinates(_ clubs: [Center], accessToken: String) async throws -> [Center] {
        guard let catalog = try? await fetchClubCatalog(accessToken: accessToken), !clubs.isEmpty else {
            return clubs
        }
        let byName = Dictionary(grouping: catalog) { Self.normalize($0.name) }
        return clubs.map { club in
            // Prefer an unambiguous exact name match; fall back to a single
            // unambiguous containment match. Ambiguity leaves the club without
            // coordinates rather than risk mapping to the wrong club.
            var candidates = byName[Self.normalize(club.clubName)] ?? []
            if candidates.count != 1 {
                let key = Self.normalize(club.clubName)
                candidates = byName
                    .filter { $0.key.contains(key) || key.contains($0.key) }
                    .flatMap(\.value)
            }
            guard let location = candidates.count == 1 ? candidates.first : nil,
                  let gps = location.gpsLocation else {
                return club
            }
            let parts = gps.split(separator: ",").compactMap { Double($0.trimmingCharacters(in: .whitespaces)) }
            guard parts.count == 2 else { return club }
            return Center(
                clubNo: club.clubNo,
                clubName: club.clubName ?? location.name,
                latitude: parts[0],
                longitude: parts[1],
                address1: location.address,
                address2: club.address2,
                address3: club.address3,
                postalCode: club.postalCode,
                urlName: club.urlName,
                title: club.title,
                description: club.description,
                telephone: club.telephone,
                regionDesc: club.regionDesc,
                autonomousRegion: club.autonomousRegion,
                isOpen: club.isOpen
            )
        }
    }

    /// Case- and accent-insensitive key for matching club names.
    private static func normalize(_ name: String?) -> String {
        (name ?? "").folding(options: [.diacriticInsensitive, .caseInsensitive], locale: Locale(identifier: "es_ES"))
    }

    private func fetchClubCatalog(accessToken: String) async throws -> [ClubLocation] {
        let data = try await httpData(
            url: url(for: "/api/v2.0/\(locale)/clubs"),
            method: "GET",
            headers: ["Authorization": "Bearer \(accessToken)"]
        )
        return try JSONDecoder().decode([ClubLocation].self, from: data)
    }

    // MARK: - Stage 1: anonymous client credentials

    private func clientCredentials() async throws -> String {
        let body: Data
        do {
            body = try JSONEncoder().encode([
                "grant_type": "client_credentials",
                "client_id": clientID,
                "client_secret": clientSecret,
            ])
        } catch {
            throw VivaGymError(message: "could not encode request", status: 500)
        }
        let data = try await httpData(
            url: url(for: "/oauth/v2/token"),
            method: "POST",
            headers: ["Content-Type": "application/json"],
            body: body
        )
        return try Self.parseTokenResponse(data, what: "client_credentials").accessToken
    }

    // MARK: - Plumbing

    private func url(for path: String) -> URL {
        URL(string: path, relativeTo: baseURL)!.absoluteURL
    }

    private func httpData(
        url: URL,
        method: String,
        headers: [String: String] = [:],
        body: Data? = nil
    ) async throws -> Data {
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.httpBody = body
        request.timeoutInterval = 30
        for (key, value) in headers {
            request.setValue(value, forHTTPHeaderField: key)
        }
        let (data, response): (Data, URLResponse)
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw VivaGymError(message: "Network error: \(error.localizedDescription)", status: 0)
        }
        guard let http = response as? HTTPURLResponse else {
            throw VivaGymError(message: "Invalid response", status: 0)
        }
        guard (200..<300).contains(http.statusCode) else {
            throw VivaGymError(message: Self.upstreamMessage(data, status: http.statusCode), status: http.statusCode)
        }
        return data
    }

    static func parseTokenResponse(_ data: Data, what: String) throws -> TokenPair {
        let pair: TokenPair
        do {
            pair = try JSONDecoder().decode(TokenPair.self, from: data)
        } catch {
            throw VivaGymError(message: "\(what) returned an invalid response", status: 502)
        }
        if pair.accessToken.isEmpty {
            throw VivaGymError(message: "\(what) returned no access_token", status: 502)
        }
        return pair
    }

    static func upstreamMessage(_ data: Data, status: Int) -> String {
        if let parsed = try? JSONDecoder().decode(UpstreamError.self, from: data) {
            if let message = parsed.message, !message.isEmpty { return message }
            if let errorDescription = parsed.errorDescription, !errorDescription.isEmpty {
                return errorDescription
            }
        }
        return "VivaGym API \(status)"
    }

    private struct UpstreamError: Codable {
        let message: String?
        let errorDescription: String?

        enum CodingKeys: String, CodingKey {
            case message
            case errorDescription = "error_description"
        }
    }
}