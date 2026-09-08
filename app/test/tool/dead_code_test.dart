@TestOn('vm')
library;

import 'package:analyzer/dart/analysis/utilities.dart';
import 'package:flutter_test/flutter_test.dart';

import '../../tool/dead_code.dart';

Map<String, int> countsOf(String source) =>
    identifierCounts(parseString(content: source, throwIfDiagnostics: false).unit);

void main() {
  group('identifierCounts', () {
    test('counts a plain identifier at every use site', () {
      final counts = countsOf('''
class Foo {
  final String bar;
  Foo(this.bar);
  String get echo => bar;
}
''');
      // Declaration + constructor initializer + getter body.
      expect(counts['bar'], 3);
      expect(counts['Foo'], 2);
    });

    test('counts contextual keywords used as names', () {
      // Regression: `source` scans as TokenType.SOURCE, not IDENTIFIER, so a
      // token-type check silently dropped every use and reported the field
      // dead. The same trap applies to `show`, `when`, `base`, `of`, `sync`.
      for (final name in <String>['source', 'show', 'when', 'base', 'of']) {
        final counts = countsOf('''
class Event {
  final String $name;
  Event(this.$name);
  Map<String, Object?> get parameters => {'k': $name};
}
''');
        expect(
          counts[name],
          3,
          reason: '"$name" is a contextual keyword and must still be counted',
        );
      }
    });

    test('ignores names that appear only in comments', () {
      final counts = countsOf('''
/// Mentions [Ghost] in a doc comment.
// And Ghost again in a line comment.
class Real {}
''');
      expect(counts['Ghost'], isNull);
      expect(counts['Real'], 1);
    });

    test('ignores names inside directives so re-exports are not uses', () {
      final counts = countsOf('''
export 'other.dart' show Reexported;
import 'x.dart' show Imported;

class Real {}
''');
      expect(counts['Reexported'], isNull);
      expect(counts['Imported'], isNull);
      expect(counts['Real'], 1);
    });

    test('ignores names that appear only inside string literals', () {
      final counts = countsOf('''
class Real {
  static const label = 'NotAnIdentifier';
}
''');
      expect(counts['NotAnIdentifier'], isNull);
    });
  });
}
