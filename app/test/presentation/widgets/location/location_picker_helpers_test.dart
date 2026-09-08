import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/location/location_picker_helpers.dart';

void main() {
  group('buildDirectionsUrl', () {
    group('iOS', () {
      test('coords produce maps.apple.com daddr lat,lng', () {
        final url = buildDirectionsUrl(
          isIOS: true,
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
          name: 'Ignored when coords present',
        );
        expect(url, 'http://maps.apple.com/?daddr=37.7749,-122.4194');
      });

      test('name-only produces maps.apple.com daddr with encoded name', () {
        final url = buildDirectionsUrl(
          isIOS: true,
          name: 'Golden Gate Park',
        );
        expect(url, 'http://maps.apple.com/?daddr=Golden%20Gate%20Park');
      });

      test('no coords and no name returns null', () {
        expect(buildDirectionsUrl(isIOS: true), isNull);
      });

      test('0,0 coords without name returns null (treated as unset)', () {
        expect(
          buildDirectionsUrl(isIOS: true, latitudeDeg: 0, longitudeDeg: 0),
          isNull,
        );
      });
    });

    group('Android', () {
      test('coords produce geo: scheme URI with q parameter', () {
        final url = buildDirectionsUrl(
          isIOS: false,
          latitudeDeg: 37.7749,
          longitudeDeg: -122.4194,
        );
        expect(url, 'geo:37.7749,-122.4194?q=37.7749,-122.4194');
      });

      test('name-only falls back to Google Maps HTTPS URL', () {
        final url = buildDirectionsUrl(
          isIOS: false,
          name: 'Golden Gate Park',
        );
        expect(
          url,
          'https://www.google.com/maps/search/?api=1'
          '&query=Golden%20Gate%20Park',
        );
      });

      test('no coords and no name returns null', () {
        expect(buildDirectionsUrl(isIOS: false), isNull);
      });

      test('whitespace-only name is treated as unset', () {
        expect(
          buildDirectionsUrl(isIOS: false, name: '   '),
          isNull,
        );
      });

      test('coords win over name', () {
        final url = buildDirectionsUrl(
          isIOS: false,
          latitudeDeg: 1,
          longitudeDeg: 2,
          name: 'should not appear',
        );
        expect(url, 'geo:1.0,2.0?q=1.0,2.0');
      });
    });
  });
}
