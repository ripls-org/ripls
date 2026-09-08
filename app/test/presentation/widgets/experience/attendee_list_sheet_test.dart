import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/experience.pbenum.dart' as proto;
import 'package:ripls/presentation/widgets/experience/attendee_list_sheet.dart';

void main() {
  group('isExperienceRsvpReadOnly', () {
    test('ACTIVE is editable', () {
      expect(
        isExperienceRsvpReadOnly(
          proto.ExperienceState.EXPERIENCE_STATE_ACTIVE,
        ),
        isFalse,
      );
    });

    test('JOINED is editable', () {
      expect(
        isExperienceRsvpReadOnly(
          proto.ExperienceState.EXPERIENCE_STATE_JOINED,
        ),
        isFalse,
      );
    });

    test('IN_PROCESS is editable (regression: issue #1080)', () {
      // The server allow-list for RSVPToExperience explicitly permits
      // IN_PROCESS so latecomers can still confirm they're going. The
      // attendee sheet must match that — otherwise users who tap RSVP
      // on an in-progress event see a read-only sheet and can't respond.
      expect(
        isExperienceRsvpReadOnly(
          proto.ExperienceState.EXPERIENCE_STATE_IN_PROCESS,
        ),
        isFalse,
      );
    });

    test('COMPLETED is read-only', () {
      expect(
        isExperienceRsvpReadOnly(
          proto.ExperienceState.EXPERIENCE_STATE_COMPLETED,
        ),
        isTrue,
      );
    });

    test('CANCELLED is read-only', () {
      expect(
        isExperienceRsvpReadOnly(
          proto.ExperienceState.EXPERIENCE_STATE_CANCELLED,
        ),
        isTrue,
      );
    });
  });
}
