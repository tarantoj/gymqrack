import XCTest

final class ClientTests: XCTestCase {
    private var client: VivaGymClient!
    private var receivedRequests: [URLRequest] = []

    override func setUp() {
        super.setUp()
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [MockURLProtocol.self]
        client = VivaGymClient(clientID: "test-cid", clientSecret: "test-csec", locale: "es", session: URLSession(configuration: config))
        receivedRequests = []
    }

    override func tearDown() {
        MockURLProtocol.handler = nil
        client = nil
        receivedRequests = []
        super.tearDown()
    }

    /// Route responses by request; record every request for assertions.
    private func installHandler(_ route: @escaping (URLRequest) -> (HTTPURLResponse, Data)) {
        MockURLProtocol.handler = { [weak self] request in
            self?.receivedRequests.append(request)
            return route(request)
        }
    }

    private func response(status: Int = 200) -> HTTPURLResponse {
        HTTPURLResponse(url: URL(string: "https://vivagym.myvitale.com/")!, statusCode: status, httpVersion: nil, headerFields: nil)!
    }

    private func json(_ string: String) -> Data { Data(string.utf8) }

    // MARK: - login

    func testLoginPerformsTwoStageOAuth() async throws {
        installHandler { request in
            switch request.url!.path {
            case "/oauth/v2/token":
                XCTAssertEqual(request.httpMethod, "POST")
                XCTAssertEqual(request.value(forHTTPHeaderField: "Content-Type"), "application/json")
                let tokenBody = String(decoding: request.httpBody ?? Data(), as: UTF8.self)
                XCTAssertTrue(tokenBody.contains("\"grant_type\":\"client_credentials\""), tokenBody)
                return (self.response(), self.json(#"{"access_token":"temp","expires_in":600}"#))
            case "/api/v2.0/es/exerp/newAuth":
                XCTAssertEqual(request.httpMethod, "POST")
                let body = String(decoding: request.httpBody ?? Data(), as: UTF8.self)
                XCTAssertTrue(body.contains("access_token=temp"), body)
                XCTAssertTrue(body.contains("appName=vivagym"), body)
                XCTAssertTrue(body.contains("email=member@example.com"), body)
                return (self.response(), self.json(#"{"access_token":"acc","refresh_token":"ref","expires_in":590}"#))
            default:
                XCTFail("unexpected path \(request.url!.path)")
                return (self.response(), Data())
            }
        }

        let pair = try await client.login(email: "member@example.com", password: "pw")
        XCTAssertEqual(pair.accessToken, "acc")
        XCTAssertEqual(pair.refreshToken, "ref")
        XCTAssertEqual(pair.expiresIn, 590)
        XCTAssertEqual(receivedRequests.count, 2)
    }

    func testLoginDefaultsExpiresInWhenMissing() async throws {
        installHandler { request in
            if request.url!.path.hasSuffix("/oauth/v2/token") {
                return (self.response(), self.json(#"{"access_token":"temp"}"#))
            }
            return (self.response(), self.json(#"{"access_token":"acc"}"#))
        }
        let pair = try await client.login(email: "a@b.c", password: "pw")
        XCTAssertNil(pair.expiresIn)
    }

    func testLoginPropagatesUpstreamError() async {
        installHandler { _ in (self.response(status: 401), self.json(#"{"message":"Invalid credentials"}"#)) }

        do {
            _ = try await client.login(email: "a@b.c", password: "nope")
            XCTFail("expected failure")
        } catch let error as VivaGymError {
            XCTAssertEqual(error.message, "Invalid credentials")
            XCTAssertEqual(error.status, 401)
        } catch {
            XCTFail("unexpected \(error)")
        }
    }

    // MARK: - refresh

    func testRefreshKeepsOldRefreshTokenWhenOmitted() async throws {
        installHandler { request in
            XCTAssertEqual(request.url!.path, "/api/email/refresh")
            XCTAssertTrue(request.url!.query!.contains("refresh_token=oldtoken"))
            return (self.response(), self.json(#"{"access_token":"newacc","expires_in":600}"#))
        }

        let pair = try await client.refresh(refreshToken: "oldtoken")
        XCTAssertEqual(pair.accessToken, "newacc")
        XCTAssertEqual(pair.refreshToken, "oldtoken")
    }

    // MARK: - QR

    func testFetchQRReturnsJsonStringPayload() async throws {
        installHandler { request in
            XCTAssertEqual(request.url!.path, "/api/v2.0/exerp/qr")
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer acc")
            return (self.response(), self.json(#""exerp:checkin:123p45678-1786821607534-0123456789abcdef0123456789abcdef""#))
        }

        let payload = try await client.fetchQR(accessToken: "acc")
        XCTAssertEqual(payload, "exerp:checkin:123p45678-1786821607534-0123456789abcdef0123456789abcdef")
    }

    func testFetchQRFallsBackToRawBody() async throws {
        installHandler { _ in (self.response(), self.json("not-JSON")) }
        let payload = try await client.fetchQR(accessToken: "acc")
        XCTAssertEqual(payload, "not-JSON")
    }

    // MARK: - clubs

    func testFetchUserClubsDecodesStringAndNumericCoordinates() async throws {
        installHandler { request in
            XCTAssertEqual(request.url!.path, "/api/v2.0/exerp/user-clubs")
            return (self.response(), self.json(Self.userClubsJSON))
        }

        let clubs = try await client.fetchUserClubs(accessToken: "acc")
        XCTAssertEqual(clubs.count, 2)

        let first = clubs[0]
        XCTAssertEqual(first.clubNo, 123)
        XCTAssertEqual(first.clubName, "VivaGym Madrid Las Rozas")
        XCTAssertEqual(first.latitude ?? -1, 40.49, accuracy: 0.001)
        XCTAssertEqual(first.longitude ?? -1, -3.87, accuracy: 0.001)

        let second = clubs[1]
        XCTAssertEqual(second.clubNo, 456)
        XCTAssertEqual(second.latitude ?? -1, 41.41, accuracy: 0.001)
        XCTAssertEqual(second.longitude ?? -1, 2.192, accuracy: 0.001)
    }

    func testCenterDecoderToleratesEmptyAndStringNumbers() throws {
        let clubs = try JSONDecoder().decode(
            [Center].self,
            from: json(Self.allClubsJSON)
        )
        XCTAssertEqual(clubs.count, 3)
        // string "40,49" (comma decimal) parses; empty stays nil.
        XCTAssertEqual(clubs[0].latitude ?? -1, 40.49, accuracy: 0.001)
        XCTAssertNil(clubs[2].coordinate)
    }

    private static let userClubsJSON = """
    [
      {
        "clubNo": 123,
        "clubName": "VivaGym Madrid Las Rozas",
        "latitude": "40.49",
        "longitude": "-3.87",
        "address1": "Av. de Europa 1",
        "postalCode": "28232",
        "urlName": "las-rozas"
      },
      {
        "clubNo": 456,
        "clubName": "VivaGym Barcelona Glòries",
        "latitude": 41.41,
        "longitude": 2.192,
        "address1": "Torre Glòries"
      }
    ]
    """

    private static let allClubsJSON = """
    [
      {
        "clubNo": 123,
        "clubName": "VivaGym Madrid Las Rozas",
        "latitude": "40,49",
        "longitude": "-3,87"
      },
      {
        "clubNo": 456,
        "clubName": "Numeric club",
        "latitude": 41.41,
        "longitude": 2.192
      },
      {
        "clubNo": 789,
        "clubName": "Missing coordinates club",
        "latitude": "",
        "longitude": ""
      }
    ]
    """
}