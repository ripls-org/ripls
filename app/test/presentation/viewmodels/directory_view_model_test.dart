import 'package:fixnum/fixnum.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart';
import 'package:ripls/l10n/app_localizations_en.dart';
import 'package:ripls/presentation/viewmodels/directory_entry.dart';
import 'package:ripls/presentation/viewmodels/directory_view_model.dart';
import 'package:ripls/services/providers/community_providers.dart';

class _StubCommunities extends CommunitiesNotifier {
  _StubCommunities(this._initial);

  final CommunitiesState _initial;

  @override
  CommunitiesState build() => _initial;
}

ProviderContainer _container(List<CommunityItem> communities) {
  final container = ProviderContainer(overrides: [
    communitiesProvider.overrideWith(
      () => _StubCommunities(CommunitiesState(communities: communities)),
    ),
  ]);
  addTearDown(container.dispose);
  return container;
}

void main() {
  final l10n = AppLocalizationsEn();

  group('directoryEntriesProvider', () {
    test('maps a named community to a community entry', () {
      final container = _container([
        CommunityItem(
          id: 'c1',
          name: 'Boulder Backcountry Crew',
          memberCount: 3,
          mediaIds: ['m1'],
          memberPreviewFirstNames: ['Thomas', 'Alfred', 'Betty', 'Devon'],
          lastActivityUnixSec: Int64(1700000000),
        ),
      ]);

      final entries = container.read(directoryEntriesProvider);
      expect(entries, hasLength(1));
      final e = entries.single;
      expect(e.kind, DirectoryKind.community);
      expect(e.displayName, 'Boulder Backcountry Crew');
      expect(e.mediaId, 'm1');
      expect(e.memberCount, 3);
      expect(e.lastActivityUnixSec, 1700000000);
      expect(e.isNameless, isFalse);
      // A named community passes straight through — localized() is a no-op.
      expect(e.localized(l10n).displayName, 'Boulder Backcountry Crew');
    });

    test('leaves a nameless group unlabelled, carrying the raw fields', () {
      // The provider has no BuildContext, and viewmodels never resolve strings
      // (docs/client/i18n.md) — so the label is the screen's job (#2937).
      final container = _container([
        CommunityItem(
          id: 'g1',
          name: '  ',
          memberCount: 4,
          memberPreviewFirstNames: ['Devon', 'Priya', 'Wes'],
          originItemName: 'Chainsaw',
        ),
      ]);

      final e = container.read(directoryEntriesProvider).single;
      expect(e.kind, DirectoryKind.group);
      expect(e.isNameless, isTrue);
      expect(e.displayName, isEmpty);
      expect(e.memberPreviewFirstNames, ['Devon', 'Priya', 'Wes']);
      expect(e.originItemName, 'Chainsaw');
    });

    test('omits absent media, activity, and origin', () {
      final container = _container([
        CommunityItem(id: 'c1', name: 'Quiet Crew'),
      ]);

      final e = container.read(directoryEntriesProvider).single;
      expect(e.mediaId, isNull);
      expect(e.lastActivityUnixSec, isNull);
      expect(e.originItemName, isNull);
      expect(e.subtitle, isNull);
    });
  });

  group('DirectoryEntry.localized', () {
    // The Directory used to derive its own group label — origin-item first, and
    // with a hardcoded-English "& N others" tail. It now shares the one
    // localized derivation with the Workshop switcher and the settings hub:
    // the people are the label, the origin item is the second line (#2937).
    test('labels a nameless group by its members, viewer first', () {
      final e = DirectoryEntry(
        id: 'g1',
        kind: DirectoryKind.group,
        displayName: '',
        memberCount: 4,
        isNameless: true,
        memberPreviewFirstNames: ['Devon', 'Priya', 'Wes'],
      ).localized(l10n);

      expect(e.displayName, 'You, Devon, Priya, and Wes');
    });

    test('folds the remainder into ", and N others"', () {
      final e = DirectoryEntry(
        id: 'g1',
        kind: DirectoryKind.group,
        displayName: '',
        memberCount: 9,
        isNameless: true,
        memberPreviewFirstNames: ['Devon', 'Priya', 'Wes'],
      ).localized(l10n);

      expect(e.displayName, 'You, Devon, Priya, Wes, and 5 others');
      // No hardcoded English tail survives.
      expect(e.displayName, isNot(contains('&')));
    });

    test('the origin item becomes the subtitle, not the label', () {
      final e = DirectoryEntry(
        id: 'g1',
        kind: DirectoryKind.group,
        displayName: '',
        memberCount: 3,
        isNameless: true,
        memberPreviewFirstNames: ['Devon', 'Priya'],
        originItemName: 'Chainsaw',
      ).localized(l10n);

      expect(e.displayName, 'You, Devon, and Priya');
      expect(e.subtitle, 'from Chainsaw');
    });

    test('no origin leaves the subtitle null for the row to fill', () {
      // _actionLine falls through to the member count.
      final e = DirectoryEntry(
        id: 'g1',
        kind: DirectoryKind.group,
        displayName: '',
        memberCount: 3,
        isNameless: true,
        memberPreviewFirstNames: ['Devon'],
      ).localized(l10n);

      expect(e.subtitle, isNull);
    });

    test('members with no names still produce a non-empty label', () {
      // The bug this whole change exists to prevent: never render blank.
      final e = DirectoryEntry(
        id: 'g1',
        kind: DirectoryKind.group,
        displayName: '',
        memberCount: 3,
        isNameless: true,
      ).localized(l10n);

      expect(e.displayName, '3 members');
    });

    test('a person entry is returned untouched', () {
      final person = DirectoryEntry(
        id: 'u1',
        kind: DirectoryKind.person,
        displayName: 'Ada',
      );
      expect(identical(person.localized(l10n), person), isTrue);
    });
  });
}
