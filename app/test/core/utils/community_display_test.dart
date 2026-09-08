import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/community_display.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart'
    show CommunityItem;
import 'package:ripls/l10n/app_localizations_en.dart';

void main() {
  final l10n = AppLocalizationsEn();

  group('communityDisplayName', () {
    test('named community shows its name verbatim', () {
      final c = CommunityItem(id: '1', name: 'Trail Crew', memberCount: 5);
      expect(communityDisplayName(c, l10n), 'Trail Crew');
    });

    test('whitespace-only name is treated as nameless', () {
      final c = CommunityItem(
        id: '1',
        name: '   ',
        memberCount: 2,
        memberPreviewFirstNames: ['Alex'],
      );
      // From the viewer's perspective ("You and Alex") so a two-person group
      // still reads as a group, not a single other person.
      expect(communityDisplayName(c, l10n), 'You and Alex');
    });

    test('three people render an Oxford-comma list (no overflow)', () {
      // memberCount 3 = viewer + 2 others, both shown.
      final c = CommunityItem(
        id: '2',
        name: '',
        memberCount: 3,
        memberPreviewFirstNames: ['Alex', 'Sam'],
      );
      expect(communityDisplayName(c, l10n), 'You, Alex, and Sam');
    });

    test('four people render an Oxford-comma list (no overflow)', () {
      // memberCount 4 = viewer + 3 others, all shown (preview cap is 3).
      final c = CommunityItem(
        id: '2b',
        name: '',
        memberCount: 4,
        memberPreviewFirstNames: ['Alex', 'Sam', 'Bo'],
      );
      expect(communityDisplayName(c, l10n), 'You, Alex, Sam, and Bo');
    });

    test('nameless community folds the remainder into ", and N others"', () {
      // memberCount 9 = viewer + 8 others, 3 shown -> 5 others.
      final c = CommunityItem(
        id: '3',
        name: '',
        memberCount: 9,
        memberPreviewFirstNames: ['Alex', 'Sam', 'Bo'],
      );
      expect(
        communityDisplayName(c, l10n),
        'You, Alex, Sam, Bo, and 5 others',
      );
    });

    test('nameless community with no usable names falls back to a count', () {
      final c = CommunityItem(
        id: '4',
        name: '',
        memberCount: 2,
        memberPreviewFirstNames: [],
      );
      expect(communityDisplayName(c, l10n), '2 members');
    });
  });

  group('communitySubtitle', () {
    test('a description wins for a named community', () {
      final c = CommunityItem(
        id: '1',
        name: 'Trail Crew',
        description: 'We maintain the ridge trail',
        memberCount: 5,
      );
      expect(communitySubtitle(c, l10n), 'We maintain the ridge trail');
    });

    test('a named community without a description gets no second line', () {
      final c = CommunityItem(id: '1', name: 'Trail Crew', memberCount: 5);
      expect(communitySubtitle(c, l10n), isNull);
    });

    test('a nameless group is identified by the item that spawned it', () {
      final c = CommunityItem(
        id: '2',
        name: '',
        memberCount: 3,
        memberPreviewFirstNames: ['Alex', 'Sam'],
        originItemName: 'Dinner at Este',
      );
      expect(communitySubtitle(c, l10n), 'from Dinner at Este');
    });

    test('a nameless group with no origin falls back to a member count', () {
      final c = CommunityItem(
        id: '3',
        name: '',
        memberCount: 3,
        memberPreviewFirstNames: ['Alex', 'Sam'],
      );
      expect(communitySubtitle(c, l10n), '3 members');
    });

    test('a description still wins over the origin item', () {
      // A nameless community that somehow carries a description: the human
      // text is more informative than the provenance.
      final c = CommunityItem(
        id: '4',
        name: '',
        description: 'Potluck crew',
        memberCount: 3,
        originItemName: 'Dinner at Este',
      );
      expect(communitySubtitle(c, l10n), 'Potluck crew');
    });

    test('nothing to show yields null rather than an empty string', () {
      final c = CommunityItem(id: '5', name: '', memberCount: 0);
      expect(communitySubtitle(c, l10n), isNull);
    });

    test('no second line when the title is already the member count', () {
      // With no preview names communityDisplayName falls back to "4 members";
      // a member-count subtitle would render the row as "4 members / 4 members".
      final c = CommunityItem(id: '6', name: '', memberCount: 4);
      expect(communityDisplayName(c, l10n), '4 members');
      expect(communitySubtitle(c, l10n), isNull);
    });
  });

  group('communityPublicName', () {
    // The label that leaves the group. A nameless community must never be
    // described to a non-member by its members (#2937).
    test('a named community is public as-is', () {
      final c = CommunityItem(id: '1', name: 'Trail Crew', memberCount: 5);
      expect(communityPublicName(c, l10n), 'Trail Crew');
    });

    test('a nameless group gets the generic label, not its members', () {
      final c = CommunityItem(
        id: '2',
        name: '',
        memberCount: 3,
        memberPreviewFirstNames: ['Alex', 'Sam'],
      );
      expect(communityPublicName(c, l10n), 'a Ripls group');
      // The in-app label leaks first names; the public one must not.
      expect(communityPublicName(c, l10n), isNot(contains('Alex')));
      expect(communityDisplayName(c, l10n), contains('Alex'));
    });
  });

  group('formatNameList', () {
    // The shared Oxford-comma joiner used by both the community group-text name
    // (viewer prepended as "You") and the Workshop subtitle (bare first names).
    test('one name', () {
      expect(formatNameList(l10n, ['Mike'], 0), 'Mike');
    });
    test('two names', () {
      expect(formatNameList(l10n, ['Mike', 'Diego'], 0), 'Mike and Diego');
    });
    test('three or more names use an Oxford comma', () {
      expect(
        formatNameList(l10n, ['Mike', 'Diego', 'Sarah'], 0),
        'Mike, Diego, and Sarah',
      );
    });
    test('overflow appends ", and N others"', () {
      expect(
        formatNameList(l10n, ['Mike', 'Diego', 'Sarah'], 8),
        'Mike, Diego, Sarah, and 8 others',
      );
    });
    test('a single overflow reads "1 other"', () {
      expect(
        formatNameList(l10n, ['Mike', 'Diego'], 1),
        'Mike, Diego, and 1 other',
      );
    });
  });

  group('communityDisplayNameFromList', () {
    final loaded = [
      CommunityItem(id: 'named', name: 'Trail Crew', memberCount: 5),
      CommunityItem(
        id: 'nameless',
        name: '',
        memberCount: 4,
        memberPreviewFirstNames: ['Alex', 'Sam'],
      ),
    ];

    test('a non-empty fallback name wins without a list lookup', () {
      expect(
        communityDisplayNameFromList(
          communityId: 'whatever',
          fallbackName: 'Book Club',
          fallbackMemberCount: 9,
          communities: const [],
          l10n: l10n,
        ),
        'Book Club',
      );
    });

    test('nameless community resolves group-text from the loaded list', () {
      // memberCount 4 = viewer + 3 others, 2 shown -> 1 other (singular).
      expect(
        communityDisplayNameFromList(
          communityId: 'nameless',
          fallbackName: '',
          fallbackMemberCount: 4,
          communities: loaded,
          l10n: l10n,
        ),
        'You, Alex, Sam, and 1 other',
      );
    });

    test('nameless community absent from the list falls back to a count', () {
      expect(
        communityDisplayNameFromList(
          communityId: 'unknown',
          fallbackName: '',
          fallbackMemberCount: 3,
          communities: loaded,
          l10n: l10n,
        ),
        '3 members',
      );
    });
  });
}
