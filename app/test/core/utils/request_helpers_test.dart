import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/request_helpers.dart';
import 'package:ripls/data/gen/ripls/api/gear.pb.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart';
import 'package:ripls/data/gen/ripls/api/transfer.pb.dart';

void main() {
  group('getRequestStateLabel', () {
    test('returns "Request Active" for ACTIVE state', () {
      final request = Request(state: RequestState.REQUEST_STATE_ACTIVE);
      expect(getRequestStateLabel(request), equals('Request Active'));
    });

    test('returns "Request Help Offered" for OFFERS_RECEIVED state', () {
      final request = Request(
        state: RequestState.REQUEST_STATE_OFFERS_RECEIVED,
      );
      expect(getRequestStateLabel(request), equals('Request Help Offered'));
    });

    test('returns "Request Fulfilled ✓" for FULFILLED state', () {
      final request = Request(state: RequestState.REQUEST_STATE_FULFILLED);
      expect(getRequestStateLabel(request), equals('Request Fulfilled ✓'));
    });

    test('returns "Request Cancelled" for CANCELLED state', () {
      final request = Request(state: RequestState.REQUEST_STATE_CANCELLED);
      expect(getRequestStateLabel(request), equals('Request Cancelled'));
    });

    test('returns "Request" for UNSPECIFIED state', () {
      final request = Request(state: RequestState.REQUEST_STATE_UNSPECIFIED);
      expect(getRequestStateLabel(request), equals('Request'));
    });
  });

  group('getTransferChipLabel', () {
    group('Owner scenarios', () {
      test('returns INQUIRIES (3) with check_circle icon for owner with 3 inquiries', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 3,
        );

        expect(result.label, equals('INQUIRIES (3)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('returns INQUIRIES (1) with check_circle icon for owner with 1 inquiry', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          requestCount: 1,
        );

        expect(result.label, equals('INQUIRIES (1)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('returns INQUIRIES (0) with circle_outlined icon for owner with 0 inquiries', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('returns INQUIRIES (0) with circle_outlined icon for owner with null requestCount',
          () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          requestCount: null,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('ignores availability type for owner (works same for loan and giveaway)',
          () {
        final loanResult = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 5,
        );

        final giveawayResult = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          requestCount: 5,
        );

        expect(loanResult.label, equals(giveawayResult.label));
        expect(loanResult.icon, equals(giveawayResult.icon));
      });

      test('uses requestCount not participantCount (giveaway with multiple participants)',
          () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          requestCount: 1, // 1 active transfer
          participantCount: 5, // ignored - participantCount includes all modal users
        );

        expect(result.label, equals('INQUIRIES (1)')); // Uses requestCount (active interests)
        expect(result.icon, equals(Icons.check_circle));
      });

      test('uses requestCount not participantCount (loan scenario)', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 2, // 2 transfer records
          participantCount: 3, // ignored
        );

        expect(result.label, equals('INQUIRIES (2)')); // Uses requestCount
        expect(result.icon, equals(Icons.check_circle));
      });

      test('uses requestCount when participantCount is null', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          requestCount: 3,
          participantCount: null,
        );

        expect(result.label, equals('INQUIRIES (3)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('uses requestCount when participantCount is 0', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 2,
          participantCount: 0,
        );

        expect(result.label, equals('INQUIRIES (2)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('handles edge case: participantCount is 1 (only owner, no inquiries)', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          requestCount: 0,
          participantCount: 1, // Only the owner (ignored)
        );

        expect(result.label, equals('INQUIRIES (0)')); // Uses requestCount = 0
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('multiple loans with single participants each (uses requestCount)', () {
        // Scenario: 3 separate loan requests
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 3, // 3 active transfer requests
          // participantCount not provided
        );

        expect(result.label, equals('INQUIRIES (3)'));
        expect(result.icon, equals(Icons.check_circle));
      });
    });

    group('Non-owner loan scenarios', () {
      test('returns INQUIRIES (0) with check_circle for non-owner with active loan request',
          () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          hasActiveRequest: true,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('returns INQUIRIES (0) with circle_outlined for non-owner without active loan request',
          () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          hasActiveRequest: false,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('returns INQUIRIES (0) with circle_outlined for non-owner with null hasActiveRequest',
          () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          hasActiveRequest: null,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('ignores isParticipant for loans (uses hasActiveRequest instead)', () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          hasActiveRequest: false,
          isParticipant: true, // Should be ignored for loans
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)')); // Uses hasActiveRequest, not isParticipant
        expect(result.icon, equals(Icons.circle_outlined));
      });
    });

    group('Non-owner giveaway scenarios', () {
      test('returns INQUIRIES (0) with check_circle for non-owner who is participant', () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          isParticipant: true,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('returns INQUIRIES (0) with circle_outlined for non-owner who is not participant', () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          isParticipant: false,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('returns INQUIRIES (0) with circle_outlined for non-owner with null isParticipant',
          () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          isParticipant: null,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('ignores hasActiveRequest for giveaways (uses isParticipant instead)',
          () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          hasActiveRequest: true, // Should be ignored for giveaways
          isParticipant: false,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)')); // Uses isParticipant, not hasActiveRequest
        expect(result.icon, equals(Icons.circle_outlined));
      });

      test('ignores participantCount (uses isParticipant for status)', () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_GIVEAWAY,
          participantCount: 5, // Should be ignored
          isParticipant: false,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)')); // Uses requestCount, not participantCount
        expect(result.icon, equals(Icons.circle_outlined));
      });
    });

    group('Edge cases', () {
      test('handles unspecified availability for non-owner (defaults to giveaway logic)',
          () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_UNSPECIFIED,
          isParticipant: true,
          requestCount: 0,
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('handles large inquiry counts for owner', () {
        final result = getTransferChipLabel(
          isOwner: true,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          requestCount: 999,
        );

        expect(result.label, equals('INQUIRIES (999)'));
        expect(result.icon, equals(Icons.check_circle));
      });

      test('handles missing optional parameters gracefully', () {
        final result = getTransferChipLabel(
          isOwner: false,
          availability: Availability.AVAILABILITY_FOR_LOAN,
          // All optional params omitted - defaults to requestCount: 0, hasActiveRequest: false
        );

        expect(result.label, equals('INQUIRIES (0)'));
        expect(result.icon, equals(Icons.circle_outlined));
      });
    });
  });

  group('getTransferInboxLabel', () {
    test('returns "1 loan request" for single loan request', () {
      expect(
        getTransferInboxLabel(
          transferType: TransferType.TRANSFER_TYPE_LOAN,
          count: 1,
        ),
        equals('1 loan request'),
      );
    });

    test('returns "3 loan requests" for multiple loan requests', () {
      expect(
        getTransferInboxLabel(
          transferType: TransferType.TRANSFER_TYPE_LOAN,
          count: 3,
        ),
        equals('3 loan requests'),
      );
    });

    test('returns "1 person interested" for single giveaway interest', () {
      expect(
        getTransferInboxLabel(
          transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
          count: 1,
        ),
        equals('1 person interested'),
      );
    });

    test('returns "4 people interested" for multiple giveaway interests', () {
      expect(
        getTransferInboxLabel(
          transferType: TransferType.TRANSFER_TYPE_GIVEAWAY,
          count: 4,
        ),
        equals('4 people interested'),
      );
    });
  });

  group('getTransferNotificationText', () {
    test('returns singular form for 1 request', () {
      expect(
        getTransferNotificationText(
          gearName: 'Ladder',
          count: 1,
        ),
        equals('You have 1 new request for Ladder'),
      );
    });

    test('returns plural form for multiple requests', () {
      expect(
        getTransferNotificationText(
          gearName: 'Mountain Bike',
          count: 5,
        ),
        equals('You have 5 new requests for Mountain Bike'),
      );
    });
  });
}
