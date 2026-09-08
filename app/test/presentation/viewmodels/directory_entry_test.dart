import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/viewmodels/directory_entry.dart';

DirectoryEntry _e(
  String id,
  String name, {
  DirectoryKind kind = DirectoryKind.community,
  int? lastActivity,
}) {
  return DirectoryEntry(
    id: id,
    kind: kind,
    displayName: name,
    lastActivityUnixSec: lastActivity,
  );
}

void main() {
  group('DirectoryView.arrange', () {
    test('alphabetical sort is case-insensitive', () {
      final out = DirectoryView.arrange(
        [_e('1', 'banana'), _e('2', 'Apple'), _e('3', 'cherry')],
        sort: DirectorySort.alphabetical,
        filter: DirectoryFilter.all,
      );
      expect(out.map((e) => e.displayName), ['Apple', 'banana', 'cherry']);
    });

    test('recent sort orders by activity desc, nulls last', () {
      final out = DirectoryView.arrange(
        [
          _e('1', 'Old', lastActivity: 100),
          _e('2', 'NoTime'),
          _e('3', 'New', lastActivity: 999),
        ],
        sort: DirectorySort.recent,
        filter: DirectoryFilter.all,
      );
      expect(out.map((e) => e.displayName), ['New', 'Old', 'NoTime']);
    });

    test('people filter keeps only persons', () {
      final out = DirectoryView.arrange(
        [
          _e('1', 'Crew', kind: DirectoryKind.community),
          _e('2', 'Betty', kind: DirectoryKind.person),
          _e('3', 'Group', kind: DirectoryKind.group),
        ],
        sort: DirectorySort.alphabetical,
        filter: DirectoryFilter.people,
      );
      expect(out.map((e) => e.displayName), ['Betty']);
    });

    test('groups filter keeps communities and ad-hoc groups, not people', () {
      final out = DirectoryView.arrange(
        [
          _e('1', 'Crew', kind: DirectoryKind.community),
          _e('2', 'Betty', kind: DirectoryKind.person),
          _e('3', 'Adhoc', kind: DirectoryKind.group),
        ],
        sort: DirectorySort.alphabetical,
        filter: DirectoryFilter.groups,
      );
      expect(out.map((e) => e.displayName), ['Adhoc', 'Crew']);
    });

    test('query narrows by display name, case-insensitive', () {
      final out = DirectoryView.arrange(
        [_e('1', 'Boulder Crew'), _e('2', 'Maple Climbers')],
        sort: DirectorySort.alphabetical,
        filter: DirectoryFilter.all,
        query: 'maple',
      );
      expect(out.map((e) => e.displayName), ['Maple Climbers']);
    });

    test('recentBucketFor classifies today / yesterday / older', () {
      final now = DateTime(2026, 6, 28, 10, 0);
      int ts(DateTime d) => d.millisecondsSinceEpoch ~/ 1000;
      expect(DirectoryView.recentBucketFor(ts(DateTime(2026, 6, 28, 1)), now),
          RecentBucket.today);
      expect(DirectoryView.recentBucketFor(ts(DateTime(2026, 6, 27, 23)), now),
          RecentBucket.yesterday);
      expect(DirectoryView.recentBucketFor(ts(DateTime(2026, 6, 20)), now),
          RecentBucket.older);
    });

    test('recentGroupKey collapses same day, splits distinct older days', () {
      final now = DateTime(2026, 6, 28, 10, 0);
      int ts(DateTime d) => d.millisecondsSinceEpoch ~/ 1000;
      final a = DirectoryView.recentGroupKey(ts(DateTime(2026, 6, 20, 9)), now);
      final b = DirectoryView.recentGroupKey(ts(DateTime(2026, 6, 20, 18)), now);
      final c = DirectoryView.recentGroupKey(ts(DateTime(2026, 6, 19, 9)), now);
      expect(a, b); // same day → same header
      expect(a, isNot(c)); // different day → different header
    });

    test('azBucket returns leading letter, # for non-letters', () {
      expect(DirectoryView.azBucket(_e('1', 'apple')), 'A');
      expect(DirectoryView.azBucket(_e('2', '  zed')), 'Z');
      expect(DirectoryView.azBucket(_e('3', '42 group')), '#');
      expect(DirectoryView.azBucket(_e('4', '')), '#');
    });
  });
}
