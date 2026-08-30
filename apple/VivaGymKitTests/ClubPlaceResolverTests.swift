import CoreLocation
import MapKit
import XCTest

/// Mock `ClubGeocoding` that resolves by name lookup in a canned table and can
/// simulate failures.
private final class MockGeocoder: ClubGeocoding, @unchecked Sendable {
    var places: [String: CLPlacemark] = [:]
    var failQuery = false
    var receivedQueries: [String] = []
    var receivedRegions: [CLRegion?] = []

    func geocodeAddressString(
        _ addressString: String,
        in region: CLRegion?,
        preferredLocale locale: Locale?
    ) async throws -> [CLPlacemark] {
        receivedQueries.append(addressString)
        receivedRegions.append(region)
        if failQuery { throw CLError(.geocodeCanceled) }
        guard let place = places[addressString] else { return [] }
        return [place]
    }
}

final class ClubPlaceResolverTests: XCTestCase {
    private var geocoder: MockGeocoder!
    private var resolver: ClubPlaceResolver!

    override func setUp() {
        super.setUp()
        geocoder = MockGeocoder()
        resolver = ClubPlaceResolver(geocoder: geocoder, regionRadius: 3000, mismatchThreshold: 2000)
    }

private func placemark(named name: String, lat: Double, lng: Double) -> CLPlacemark {
    MKPlacemark(coordinate: CLLocationCoordinate2D(latitude: lat, longitude: lng))
}

    private func club(
        no: Int = 1,
        name: String? = "VivaGym Troya",
        lat: Double = 39.4699,
        lng: Double = -0.3763,
        address1: String? = "Carrer de Xàtiva 8",
        postalCode: String? = "46004",
        region: String? = "València"
    ) -> Center {
        Center(
            clubNo: no,
            clubName: name,
            latitude: lat,
            longitude: lng,
            address1: address1,
            postalCode: postalCode,
            regionDesc: region
        )
    }

    // MARK: - Query building

    func testQueryIncludesNameAndAddressFields() {
        let query = ClubPlaceResolver.query(for: club())
        XCTAssertEqual(query, "VivaGym Troya, Carrer de Xàtiva 8, 46004, València")
    }

    func testQueryFallsBackToNameWhenAddressMissing() {
        let club = club(address1: nil, postalCode: nil, region: nil)
        XCTAssertEqual(ClubPlaceResolver.query(for: club), "VivaGym Troya")
    }

    func testQueryUsesAutonomousRegionWhenRegionDescMissing() {
        let club = Center(
            clubNo: 1,
            clubName: "VivaGym Ruzafa",
            latitude: 39.4644,
            longitude: -0.3763,
            autonomousRegion: "Comunitat Valenciana"
        )
        XCTAssertEqual(ClubPlaceResolver.query(for: club), "VivaGym Ruzafa, Comunitat Valenciana")
    }

    // MARK: - Resolution

    func testResolveAdoptsGeocodedCoordinate() async {
        let target = placemark(named: "VivaGym Troya", lat: 39.4702, lng: -0.3750)
        geocoder.places["VivaGym Troya, Carrer de Xàtiva 8, 46004, València"] = target

        let place = await resolver.resolve(club())

        XCTAssertNotNil(place)
        XCTAssertEqual(place?.clubNo, 1)
        XCTAssertEqual(place?.latitude ?? 0, 39.4702, accuracy: 1e-9)
        XCTAssertEqual(place?.longitude ?? 0, -0.3750, accuracy: 1e-9)
        XCTAssertEqual(place?.name, "VivaGym Troya")
        XCTAssertNotNil(place?.apiDistance)
    }

    func testResolveUsesApiCoordinateAsRegionCenter() async {
        let club = club(lat: 41.9544, lng: 2.8087)
        let target = placemark(named: "VivaGym", lat: 41.9544, lng: 2.8087)
        geocoder.places["VivaGym Troya, Carrer de Xàtiva 8, 46004, València"] = target

        _ = await resolver.resolve(club)

        let region = geocoder.receivedRegions.first as? CLCircularRegion
        XCTAssertNotNil(region)
        XCTAssertEqual(region?.center.latitude ?? 0, 41.9544, accuracy: 1e-9)
        XCTAssertEqual(region?.center.longitude ?? 0, 2.8087, accuracy: 1e-9)
        XCTAssertEqual(region?.radius ?? 0, 3000)
    }

    func testResolveReturnsNilWhenGeocodingFails() async {
        geocoder.failQuery = true
        let place = await resolver.resolve(club())
        XCTAssertNil(place)
    }

    func testResolveReturnsNilWhenNoMatch() async {
        let place = await resolver.resolve(club())
        XCTAssertNil(place)
    }

    func testResolveReturnsNilWhenClubHasNoCoordinate() async {
        let club = club(lat: 0, lng: 0)
        let noCoordinate = Center(
            clubNo: club.clubNo,
            clubName: club.clubName,
            latitude: nil,
            longitude: nil,
            address1: club.address1
        )
        let place = await resolver.resolve(noCoordinate)
        XCTAssertNil(place)
    }

    func testResolveAllReturnsOnlySuccesses() async {
        let target = placemark(named: "VivaGym", lat: 39.47, lng: -0.375)
        geocoder.places["VivaGym Troya, Carrer de Xàtiva 8, 46004, València"] = target
        // The second club has no registered place, so it fails to resolve.
        let clubs = [club(no: 1), club(no: 2, name: "VivaGym Cánovas", address1: "Paseo del Parque 1", postalCode: "29001", region: "Málaga")]

        let places = await resolver.resolveAll(clubs)

        XCTAssertEqual(places.count, 1)
        XCTAssertEqual(places.first?.clubNo, 1)
    }

    // MARK: - Mismatch logging

    func testResolveAdoptsCoordinateBeyondMismatchThreshold() async {
        // The geocoded place is ~4 km from the API coordinate (> 2000 m threshold).
        let target = placemark(named: "VivaGym Troya", lat: 39.42, lng: -0.31)
        geocoder.places["VivaGym Troya, Carrer de Xàtiva 8, 46004, València"] = target

        let place = await resolver.resolve(club())

        XCTAssertNotNil(place)
        XCTAssertEqual(place?.latitude ?? 0, 39.42, accuracy: 1e-9)
    }
}

// MARK: - Center place application

final class CenterPlaceApplicationTests: XCTestCase {
    func testApplyingPlacesOverridesMatchingClubCoordinates() {
        let clubs = [
            Center(clubNo: 1, clubName: "A", latitude: 39.4699, longitude: -0.3763),
            Center(clubNo: 2, clubName: "B", latitude: 41.9544, longitude: 2.8087),
        ]
        let places = [ClubPlace(clubNo: 1, name: "A", latitude: 39.4702, longitude: -0.3750, apiDistance: 150)]

        let applied = Center.applyingPlaces(clubs, places: places)

        XCTAssertEqual(applied[0].latitude ?? 0, 39.4702, accuracy: 1e-9)
        XCTAssertEqual(applied[0].longitude ?? 0, -0.3750, accuracy: 1e-9)
        // No place for club 2: API coordinates kept.
        XCTAssertEqual(applied[1].latitude ?? 0, 41.9544, accuracy: 1e-9)
        XCTAssertEqual(applied[1].longitude ?? 0, 2.8087, accuracy: 1e-9)
    }
}