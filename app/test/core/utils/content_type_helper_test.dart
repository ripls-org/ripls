import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/content_type_helper.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart';

void main() {
  group('ContentTypeHelper', () {
    group('getGearIcon', () {
      test('returns handshake icon for loan availability', () {
        final icon = ContentTypeHelper.getGearIcon(
          Availability.AVAILABILITY_FOR_LOAN,
        );
        expect(icon, equals(Icons.handshake));
      });

      test('returns volunteer_activism icon for giveaway availability', () {
        final icon = ContentTypeHelper.getGearIcon(
          Availability.AVAILABILITY_FOR_GIVEAWAY,
        );
        expect(icon, equals(Icons.volunteer_activism));
      });

      test('returns inventory_2 icon for unspecified availability', () {
        final icon = ContentTypeHelper.getGearIcon(
          Availability.AVAILABILITY_UNSPECIFIED,
        );
        expect(icon, equals(Icons.inventory_2));
      });

      test('returns inventory_2 icon for null availability', () {
        final icon = ContentTypeHelper.getGearIcon(null);
        expect(icon, equals(Icons.inventory_2));
      });
    });

    group('getGearLabel', () {
      test('returns LOAN label for loan availability', () {
        final label = ContentTypeHelper.getGearLabel(
          Availability.AVAILABILITY_FOR_LOAN,
        );
        expect(label, equals('LOAN'));
      });

      test('returns GIVEAWAY label for giveaway availability', () {
        final label = ContentTypeHelper.getGearLabel(
          Availability.AVAILABILITY_FOR_GIVEAWAY,
        );
        expect(label, equals('GIVEAWAY'));
      });

      test('returns GEAR label for unspecified availability', () {
        final label = ContentTypeHelper.getGearLabel(
          Availability.AVAILABILITY_UNSPECIFIED,
        );
        expect(label, equals('GEAR'));
      });

      test('returns GEAR label for null availability', () {
        final label = ContentTypeHelper.getGearLabel(null);
        expect(label, equals('GEAR'));
      });
    });

    group('getRequestIcon', () {
      test('returns help_outline icon', () {
        final icon = ContentTypeHelper.getRequestIcon();
        expect(icon, equals(Icons.help_outline));
      });
    });

    group('getRequestLabel', () {
      test('returns REQUEST label', () {
        final label = ContentTypeHelper.getRequestLabel();
        expect(label, equals('REQUEST'));
      });
    });

    group('getCommunityIcon', () {
      test('returns groups icon', () {
        final icon = ContentTypeHelper.getCommunityIcon();
        expect(icon, equals(Icons.groups));
      });
    });

    group('getCommunityLabel', () {
      test('returns Community label', () {
        final label = ContentTypeHelper.getCommunityLabel();
        expect(label, equals('Community'));
      });
    });
  });
}
