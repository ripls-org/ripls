// Dead-code detector for the Flutter app.
//
// Dart has no whole-program dead-code analysis for public API the way Go's
// `deadcode` does, so this tool reconstructs one from the parsed AST:
//
//   1. File reachability -- builds the import/export/part graph over app/lib
//      and app/test, then walks it from the real entry points. A library that
//      main.dart cannot reach is dead product code even if other dead files
//      import it (mutually-importing dead islands are invisible to a plain
//      "is this file ever imported?" grep).
//   2. Symbol reachability -- indexes every identifier token in every file,
//      ignoring comments, strings and directive combinators, then reports
//      public top-level declarations whose name never appears outside their
//      own library.
//
// Both passes are deliberately conservative: name collisions make a symbol
// look used, never unused, so findings are worth acting on and misses are the
// expected failure mode. Run it again after deleting -- removals cascade.
//
// A file containing `// dead-code-allow` anywhere in its first 40 lines is
// skipped, for the rare case where something really is reachable by means this
// tool cannot see. Pair it with a tracking issue, as with the file-size gates.
//
// Usage (from app/):
//   dart run tool/dead_code.dart              # human-readable report
//   dart run tool/dead_code.dart --json       # machine-readable
//   dart run tool/dead_code.dart --files      # file reachability only
//   dart run tool/dead_code.dart --symbols    # symbol reachability only
//   dart run tool/dead_code.dart --check      # exit 1 on any finding (CI gate)
//   dart run tool/dead_code.dart --spans      # byte ranges of dead members
import 'dart:convert';
import 'dart:io';

import 'package:analyzer/dart/analysis/features.dart';
import 'package:analyzer/dart/analysis/utilities.dart';
import 'package:analyzer/dart/ast/ast.dart';
import 'package:analyzer/dart/ast/token.dart';
import 'package:analyzer/dart/ast/visitor.dart';
import 'package:analyzer/source/line_info.dart';
import 'package:path/path.dart' as p;

/// Package name used by `package:` imports that point back into this app.
const String kPackageName = 'ripls';

/// Entry points that make a library live in a shipped build.
const List<String> kProductionEntryPoints = <String>['lib/main.dart'];

/// Paths whose contents are generated and are therefore never hand-deleted.
///
/// Any directory named `gen` counts, not just `lib/data/gen`: the design
/// tokens live in `lib/core/theme/gen/` and are rewritten by
/// `scripts/gen_design_tokens.js`, so reporting an unused token there would be
/// asking a human to edit a generated file.
bool isGenerated(String relPath) {
  final List<String> segments = relPath.split('/');
  return segments.contains('gen') ||
      relPath.endsWith('.g.dart') ||
      relPath.endsWith('.gen.dart') ||
      relPath.endsWith('.freezed.dart') ||
      relPath.endsWith('.mocks.dart') ||
      relPath.startsWith('lib/l10n/app_localizations');
}

/// One parsed Dart file plus the edges and names extracted from it.
class DartFile {
  DartFile(this.relPath);

  final String relPath;

  /// Files this one pulls in: imports, exports, parts and the reverse edge
  /// from a part back to its library.
  final Set<String> edges = <String>{};

  /// `part of` target, if this file is a part.
  String? partOf;

  /// Set when the file carries a `// dead-code-allow` opt-out near the top.
  bool allowed = false;

  /// Public top-level declarations, keyed by name.
  final List<Declaration> declarations = <Declaration>[];

  /// Public instance/static members declared on classes and mixins here.
  final List<Declaration> members = <Declaration>[];

  /// Identifier tokens that appear in real code (not comments, not strings,
  /// not `show`/`hide` combinators), with occurrence counts.
  final Map<String, int> identifiers = <String, int>{};

  /// Type names written inside a type-argument list, e.g. the `Foo` and `Bar`
  /// in `NotifierProvider<Foo, Bar>`. Such a name is part of a generic's
  /// public surface even when it is only written once, in its own library.
  final Set<String> typeArguments = <String>{};

  bool get isPart => partOf != null;
  bool get isTest => relPath.startsWith('test/');

  /// A file `flutter test` actually runs. Everything else under test/ is a
  /// helper and is only alive if a suite imports it.
  bool get isTestSuite => isTest && relPath.endsWith('_test.dart');
  bool get generated => isGenerated(relPath);
}

/// A public top-level declaration that could turn out to be unreachable.
class Declaration {
  Declaration(this.name, this.kind, this.line);

  final String name;
  final String kind;
  final int line;
}

void main(List<String> args) {
  final bool wantJson = args.contains('--json');
  final bool check = args.contains('--check');
  final bool onlyFiles = args.contains('--files');
  final bool onlySymbols = args.contains('--symbols');
  final bool doFiles = !onlySymbols;
  final bool doSymbols = !onlyFiles;

  final Directory appRoot = Directory.current;
  if (!Directory(p.join(appRoot.path, 'lib')).existsSync()) {
    stderr.writeln('error: run from the app/ directory');
    exit(2);
  }

  final Map<String, DartFile> files = _parseAll(appRoot);

  // Index every identifier to the set of libraries that mention it, and count
  // mentions within each library. Parts are folded into their library on both
  // sides: a mixin declared in a part and applied in the parent file is used,
  // not dead, and a generated part referencing its source class is not an
  // outside use.
  final Map<String, Set<String>> useSites = <String, Set<String>>{};
  final Map<String, Map<String, int>> libraryUses = <String, Map<String, int>>{};
  final Map<String, Set<String>> libraryTypeArgs = <String, Set<String>>{};
  for (final DartFile f in files.values) {
    final String owner = _libraryOf(f, files);
    final Map<String, int> own =
        libraryUses.putIfAbsent(owner, () => <String, int>{});
    for (final MapEntry<String, int> id in f.identifiers.entries) {
      useSites.putIfAbsent(id.key, () => <String>{}).add(owner);
      own[id.key] = (own[id.key] ?? 0) + id.value;
    }
    libraryTypeArgs
        .putIfAbsent(owner, () => <String>{})
        .addAll(f.typeArguments);
  }

  // Whole-app occurrence counts, used for members (see below).
  final Map<String, int> allUses = <String, int>{};
  for (final DartFile f in files.values) {
    for (final MapEntry<String, int> id in f.identifiers.entries) {
      allUses[id.key] = (allUses[id.key] ?? 0) + id.value;
    }
  }

  final Set<String> prodReachable = _reachable(
    files,
    kProductionEntryPoints.where(files.containsKey),
  );
  final Set<String> testReachable = _reachable(
    files,
    files.keys.where((String k) => files[k]!.isTestSuite),
  );

  final List<Map<String, Object?>> orphans = <Map<String, Object?>>[];
  final List<Map<String, Object?>> testOnly = <Map<String, Object?>>[];
  final List<Map<String, Object?>> orphanHelpers = <Map<String, Object?>>[];

  if (doFiles) {
    for (final DartFile f in files.values) {
      if (f.isPart || f.generated || f.allowed) continue;
      final Map<String, Object?> entry = <String, Object?>{
        'path': f.relPath,
        'lines': _lineCount(appRoot, f.relPath),
      };
      if (f.isTest) {
        // A suite is its own entry point; a helper needs a suite to import it.
        if (!f.isTestSuite && !testReachable.contains(f.relPath)) {
          orphanHelpers.add(entry);
        }
        continue;
      }
      if (prodReachable.contains(f.relPath)) continue;
      final bool reachedByTest = testReachable.contains(f.relPath);
      if (reachedByTest) {
        entry['tests'] = _testsReaching(f.relPath, files).toList()..sort();
      }
      (reachedByTest ? testOnly : orphans).add(entry);
    }
    int byPath(Map<String, Object?> a, Map<String, Object?> b) =>
        (a['path']! as String).compareTo(b['path']! as String);
    orphans.sort(byPath);
    testOnly.sort(byPath);
    orphanHelpers.sort(byPath);
  }

  final List<Map<String, Object?>> deadSymbols = <Map<String, Object?>>[];
  final List<Map<String, Object?>> privatize = <Map<String, Object?>>[];
  final List<Map<String, Object?>> deadMembers = <Map<String, Object?>>[];

  if (doSymbols) {
    for (final DartFile f in files.values) {
      if (f.generated || f.allowed) continue;
      // Files already flagged as whole-file dead do not need symbol noise.
      if (!f.isTest && !prodReachable.contains(f.relPath)) continue;
      final String owner = _libraryOf(f, files);
      for (final Declaration d in f.declarations) {
        final Set<String> sites = useSites[d.name] ?? const <String>{};
        final bool usedOutside = sites.any((String s) => s != owner);
        if (usedOutside) continue;
        final Map<String, Object?> entry = <String, Object?>{
          'path': f.relPath,
          'line': d.line,
          'name': d.name,
          'kind': d.kind,
        };
        // One mention inside the library is the declaration itself; more than
        // one means it is genuinely used, just never outside.
        if ((libraryUses[owner]?[d.name] ?? 0) > 1) {
          // A name written as a type argument -- `NotifierProvider<Foo, Bar>`
          // -- leaks out through the generic even though no other library
          // spells it, so it cannot be privatized.
          if (libraryTypeArgs[owner]?.contains(d.name) ?? false) continue;
          // Test-local fakes and builders are conventionally public; naming
          // them with a leading underscore buys nothing.
          if (f.isTest) continue;
          privatize.add(entry);
        } else {
          deadSymbols.add(entry);
        }
      }
      // Members are name-matched across the whole app, not per library: a
      // method is normally called on an instance, so its declaring library is
      // not where you would look for the call site.
      for (final Declaration d in f.members) {
        final int total = allUses[d.name] ?? 0;
        // One occurrence is the declaration itself.
        if (total > 1) continue;
        deadMembers.add(<String, Object?>{
          'path': f.relPath,
          'line': d.line,
          'name': d.name,
          'kind': d.kind,
        });
      }
    }
    int byPath(Map<String, Object?> a, Map<String, Object?> b) {
      final int c = (a['path']! as String).compareTo(b['path']! as String);
      return c != 0 ? c : (a['line']! as int).compareTo(b['line']! as int);
    }

    deadSymbols.sort(byPath);
    privatize.sort(byPath);
    deadMembers.sort(byPath);
  }

  final Map<String, Object?> report = <String, Object?>{
    'scanned': files.length,
    'orphanFiles': orphans,
    'testOnlyFiles': testOnly,
    'orphanTestHelpers': orphanHelpers,
    'deadSymbols': deadSymbols,
    'deadMembers': deadMembers,
    'privatizeCandidates': privatize,
  };

  // `privatizeCandidates` is advisory -- a file-local public name is a style
  // question, not dead weight -- so it never fails the gate.
  final int findings = orphans.length +
      testOnly.length +
      orphanHelpers.length +
      deadSymbols.length;

  if (args.contains('--spans')) {
    stdout.writeln(
      const JsonEncoder.withIndent('  ').convert(_removableSpans(
        appRoot,
        deadMembers,
      )),
    );
    return;
  }

  if (wantJson) {
    stdout.writeln(const JsonEncoder.withIndent('  ').convert(report));
  } else {
    _printReport(
      report,
      doFiles: doFiles,
      doSymbols: doSymbols,
      // The gate never fails on the advisory bucket, so printing it in CI
      // would just be scrollback between a developer and the real findings.
      showAdvisory: !check,
    );
  }

  if (check && findings > 0) {
    stderr.writeln(
      '\n$findings dead-code finding(s). Delete them, or -- if something is '
      'reachable in a way this tool cannot see -- add a `// dead-code-allow` '
      'comment near the top of the file with a tracking-issue link.',
    );
    exit(1);
  }
}

Map<String, DartFile> _parseAll(Directory appRoot) {
  final Map<String, DartFile> files = <String, DartFile>{};
  for (final String root in <String>['lib', 'test']) {
    final Directory dir = Directory(p.join(appRoot.path, root));
    if (!dir.existsSync()) continue;
    for (final FileSystemEntity e in dir.listSync(recursive: true)) {
      if (e is! File || !e.path.endsWith('.dart')) continue;
      final String rel = p.relative(e.path, from: appRoot.path);
      final DartFile f = DartFile(rel);
      files[rel] = f;
      _analyze(f, e, appRoot);
    }
  }
  // Add the reverse part -> library edge now that every file is known.
  for (final DartFile f in files.values) {
    final String? parent = f.partOf;
    if (parent != null && files.containsKey(parent)) {
      files[parent]!.edges.add(f.relPath);
    }
  }
  return files;
}

/// Marker that exempts a file from every bucket, for the rare case where
/// something is reachable in a way this tool cannot see.
const String kAllowMarker = 'dead-code-allow';

void _analyze(DartFile f, File file, Directory appRoot) {
  try {
    f.allowed = file
        .readAsLinesSync()
        .take(40)
        .any((String l) => l.contains(kAllowMarker));
  } catch (_) {
    // Unreadable file: fall through to the parse, which will bail too.
  }

  final CompilationUnit unit;
  try {
    unit = parseFile(
      path: file.path,
      featureSet: FeatureSet.latestLanguageVersion(),
      throwIfDiagnostics: false,
    ).unit;
  } catch (_) {
    return;
  }

  for (final Directive d in unit.directives) {
    if (d is PartOfDirective) {
      final String? uri = d.uri?.stringValue;
      if (uri != null) {
        final String? resolved = _resolve(uri, f.relPath);
        if (resolved != null) f.partOf = resolved;
      }
      continue;
    }
    final List<String> uris = <String>[];
    if (d is UriBasedDirective) {
      final String? uri = d.uri.stringValue;
      if (uri != null) uris.add(uri);
    }
    if (d is NamespaceDirective) {
      for (final Configuration c in d.configurations) {
        final String? uri = c.uri.stringValue;
        if (uri != null) uris.add(uri);
      }
    }
    for (final String uri in uris) {
      final String? resolved = _resolve(uri, f.relPath);
      if (resolved != null) f.edges.add(resolved);
    }
  }

  final LineInfo lineInfo = unit.lineInfo;
  for (final CompilationUnitMember m in unit.declarations) {
    for (final (String name, String kind, int offset) in _declaredNames(m)) {
      if (name.startsWith('_')) continue;
      f.declarations.add(
        Declaration(name, kind, lineInfo.getLocation(offset).lineNumber),
      );
    }
    for (final (String name, String kind, int offset) in _memberNames(m)) {
      if (name.startsWith('_')) continue;
      f.members.add(
        Declaration(name, kind, lineInfo.getLocation(offset).lineNumber),
      );
    }
  }

  // Walk the token stream (comments are attached to tokens, not in the
  // stream, so doc-comment references correctly do not count as uses).
  unit.accept(_TypeArgumentCollector(f.typeArguments));

  identifierCounts(unit).forEach((String name, int n) {
    f.identifiers[name] = (f.identifiers[name] ?? 0) + n;
  });
}

/// Matches tokens that could be a Dart identifier.
///
/// Deliberately keyed on the lexeme rather than `TokenType.IDENTIFIER`.
/// Dart's contextual keywords scan as their own token type even where they are
/// plainly being used as names — a field called `source` is a `TokenType.SOURCE`
/// token, not an identifier — so a type check silently drops every use of
/// `source`, `show`, `when`, `base`, `sync`, `of`, and the rest of that set.
///
/// Matching reserved words like `class` too is harmless: nothing is declared
/// under those names, so the extra counts land on keys nobody looks up, and
/// erring toward "used" is the safe direction for a dead-code report.
final RegExp kIdentifierLike = RegExp(r'^[A-Za-z_$][A-Za-z0-9_$]*$');

/// Counts identifier-shaped tokens in [unit], keyed by lexeme.
///
/// Comments are attached to tokens rather than living in the stream, so a name
/// that only appears in a doc comment correctly does not count as a use.
/// Directive spans are excluded for the same reason: `export 'x.dart' show Foo`
/// re-exports `Foo`, it does not use it.
Map<String, int> identifierCounts(CompilationUnit unit) {
  final List<(int, int)> directiveRanges = <(int, int)>[
    for (final Directive d in unit.directives) (d.offset, d.end),
  ];
  final Map<String, int> counts = <String, int>{};
  Token? t = unit.beginToken;
  while (t != null && t.type != TokenType.EOF) {
    if (kIdentifierLike.hasMatch(t.lexeme)) {
      final bool inDirective = directiveRanges.any(
        (r) => t!.offset >= r.$1 && t.offset < r.$2,
      );
      if (!inDirective) counts[t.lexeme] = (counts[t.lexeme] ?? 0) + 1;
    }
    if (t.next == t) break;
    t = t.next;
  }
  return counts;
}

/// Public names a top-level member declares, with the kind label used in the
/// report. Extensions are skipped: an extension's own name is almost never
/// written at a use site, so absence is not evidence of death.
Iterable<(String, String, int)> _declaredNames(CompilationUnitMember m) sync* {
  if (m is TopLevelVariableDeclaration) {
    for (final VariableDeclaration v in m.variables.variables) {
      yield (v.name.lexeme, 'variable', v.name.offset);
    }
    return;
  }
  final Token? name = _nameToken(m);
  if (name == null) return;

  final String kind;
  if (m is ClassDeclaration) {
    kind = 'class';
  } else if (m is MixinDeclaration) {
    kind = 'mixin';
  } else if (m is EnumDeclaration) {
    kind = 'enum';
  } else if (m is ExtensionTypeDeclaration) {
    kind = 'extension type';
  } else if (m is TypeAlias) {
    kind = 'typedef';
  } else if (m is FunctionDeclaration) {
    kind = m.isGetter
        ? 'getter'
        : m.isSetter
            ? 'setter'
            : 'function';
  } else {
    kind = 'declaration';
  }
  yield (name.lexeme, kind, name.offset);
}

/// Collects every type name written inside a type-argument list.
class _TypeArgumentCollector extends RecursiveAstVisitor<void> {
  _TypeArgumentCollector(this.out);

  final Set<String> out;
  int _depth = 0;

  @override
  void visitTypeArgumentList(TypeArgumentList node) {
    _depth++;
    super.visitTypeArgumentList(node);
    _depth--;
  }

  @override
  void visitNamedType(NamedType node) {
    if (_depth > 0) out.add(node.name.lexeme);
    super.visitNamedType(node);
  }
}

/// Ranges that can be cut to delete the reported dead members.
///
/// Offsets are UTF-16 code units, as everywhere else in the analyzer. A
/// consumer that indexes by Unicode code point (Python, Go) must convert, or a
/// single astral-plane character earlier in the file — an emoji in a log line
/// is enough — will shift the cut.
///
/// A member is only emitted when *every* public name it declares is dead, so a
/// `final a, b;` with one live variable is reported but left for a human. The
/// span starts at the member's own offset, which in the analyzer AST already
/// covers its doc comment and annotations.
Map<String, List<List<int>>> _removableSpans(
  Directory appRoot,
  List<Map<String, Object?>> deadMembers,
) {
  final Map<String, Set<String>> deadByPath = <String, Set<String>>{};
  for (final Map<String, Object?> e in deadMembers) {
    deadByPath
        .putIfAbsent(e['path']! as String, () => <String>{})
        .add(e['name']! as String);
  }

  final Map<String, List<List<int>>> spans = <String, List<List<int>>>{};
  deadByPath.forEach((String relPath, Set<String> dead) {
    final CompilationUnit unit;
    try {
      unit = parseFile(
        path: p.join(appRoot.path, relPath),
        featureSet: FeatureSet.latestLanguageVersion(),
        throwIfDiagnostics: false,
      ).unit;
    } catch (_) {
      return;
    }
    final List<List<int>> out = <List<int>>[];
    for (final CompilationUnitMember m in unit.declarations) {
      for (final (ClassMember member, List<String> names)
          in _memberDeclarations(m)) {
        final List<String> public =
            names.where((String n) => !n.startsWith('_')).toList();
        if (public.isEmpty) continue;
        if (!public.every(dead.contains)) continue;
        out.add(<int>[member.offset, member.end]);
      }
    }
    if (out.isNotEmpty) {
      out.sort((List<int> a, List<int> b) => a[0].compareTo(b[0]));
      spans[relPath] = out;
    }
  });
  return spans;
}

/// The members a class, mixin, enum or extension body declares, or null for a
/// declaration that has no member list at all.
///
/// analyzer 13 moved the member list behind a body node: `<decl>.members`
/// became `<decl>.body.members`. Enums carry an [EnumBody] and the rest a
/// [ClassBody]; both expose `members`, but they share no common supertype that
/// does, so each arm has to read it separately.
List<ClassMember>? _bodyMembers(CompilationUnitMember m) => switch (m) {
      ClassDeclaration() => m.body.members,
      MixinDeclaration() => m.body.members,
      EnumDeclaration() => m.body.members,
      ExtensionDeclaration() => m.body.members,
      _ => null,
    };

/// Each class member paired with every name it declares.
Iterable<(ClassMember, List<String>)> _memberDeclarations(
  CompilationUnitMember m,
) sync* {
  final List<ClassMember>? body = _bodyMembers(m);
  if (body == null) return;
  for (final ClassMember member in body) {
    if (member.metadata.any((Annotation a) => a.name.name == 'override')) {
      continue;
    }
    if (member is MethodDeclaration) {
      if (member.isOperator) continue;
      yield (member, <String>[member.name.lexeme]);
    } else if (member is FieldDeclaration) {
      yield (
        member,
        <String>[for (final v in member.fields.variables) v.name.lexeme],
      );
    }
  }
}

/// Public members a class, mixin, enum or extension declares.
///
/// Skips anything carrying `@override` — that member exists to satisfy a
/// supertype's contract, so the absence of a same-file call site says nothing.
/// Skips constructors and operators for the same reason: they are invoked
/// through the type, not by name.
Iterable<(String, String, int)> _memberNames(CompilationUnitMember m) sync* {
  final List<ClassMember>? body = _bodyMembers(m);
  if (body == null) return;
  // An extension's own name is almost never written at a use site, so
  // [_nameToken] deliberately returns null for it; label it by kind instead.
  final String owner = m is ExtensionDeclaration
      ? 'extension'
      : _nameToken(m)?.lexeme ?? '?';

  for (final ClassMember member in body) {
    final bool overrides = member.metadata
        .any((Annotation a) => a.name.name == 'override');
    if (overrides) continue;
    if (member is MethodDeclaration) {
      if (member.isOperator) continue;
      final String kind = member.isGetter
          ? 'getter'
          : member.isSetter
              ? 'setter'
              : 'method';
      yield (member.name.lexeme, '$owner.$kind', member.name.offset);
    } else if (member is FieldDeclaration) {
      for (final VariableDeclaration v in member.fields.variables) {
        yield (v.name.lexeme, '$owner.field', v.name.offset);
      }
    }
  }
}

/// The name token of a top-level declaration, or null if it has none.
///
/// Returns null for extensions, which is what we want: an extension's own name
/// is almost never written at a use site, so its absence is not evidence of
/// death.
///
/// analyzer 13 removed `NamedCompilationUnitMember`, the supertype that used to
/// expose `name` generically, so the name is read per declaration kind. The
/// type-shaped declarations moved theirs behind a [ClassNamePart]; functions
/// and typedefs still carry a bare token.
Token? _nameToken(CompilationUnitMember m) => switch (m) {
      ClassDeclaration() => m.namePart.typeName,
      EnumDeclaration() => m.namePart.typeName,
      ExtensionTypeDeclaration() => m.namePart.typeName,
      MixinDeclaration() => m.name,
      TypeAlias() => m.name,
      FunctionDeclaration() => m.name,
      _ => null,
    };

/// Resolves a directive URI to a repo-relative path, or null if it points
/// outside this package (dart:, other packages, assets).
String? _resolve(String uri, String fromRelPath) {
  if (uri.startsWith('package:$kPackageName/')) {
    return 'lib/${uri.substring('package:$kPackageName/'.length)}';
  }
  if (uri.contains(':')) return null;
  return p.normalize(p.join(p.dirname(fromRelPath), uri));
}

/// The library a file belongs to -- itself, or its parent if it is a part.
String _libraryOf(DartFile f, Map<String, DartFile> files) {
  final String? parent = f.partOf;
  if (parent != null && files.containsKey(parent)) return parent;
  return f.relPath;
}

Set<String> _reachable(Map<String, DartFile> files, Iterable<String> roots) {
  final Set<String> seen = <String>{};
  final List<String> queue = roots.toList();
  seen.addAll(queue);
  while (queue.isNotEmpty) {
    final DartFile? f = files[queue.removeLast()];
    if (f == null) continue;
    for (final String next in f.edges) {
      if (seen.add(next)) queue.add(next);
    }
  }
  return seen;
}

/// Test files that transitively reach [target] -- the tests that would have to
/// be deleted along with it.
Set<String> _testsReaching(String target, Map<String, DartFile> files) {
  final Set<String> out = <String>{};
  for (final DartFile t in files.values) {
    if (!t.isTestSuite || t.isPart) continue;
    if (_reachable(files, <String>[t.relPath]).contains(target)) {
      out.add(t.relPath);
    }
  }
  return out;
}

int _lineCount(Directory appRoot, String relPath) {
  try {
    return File(p.join(appRoot.path, relPath)).readAsLinesSync().length;
  } catch (_) {
    return 0;
  }
}

void _printReport(
  Map<String, Object?> report, {
  required bool doFiles,
  required bool doSymbols,
  required bool showAdvisory,
}) {
  final List<Map<String, Object?>> orphans =
      (report['orphanFiles']! as List<Object?>).cast<Map<String, Object?>>();
  final List<Map<String, Object?>> testOnly =
      (report['testOnlyFiles']! as List<Object?>).cast<Map<String, Object?>>();
  final List<Map<String, Object?>> helpers = (report['orphanTestHelpers']!
          as List<Object?>)
      .cast<Map<String, Object?>>();
  final List<Map<String, Object?>> dead =
      (report['deadSymbols']! as List<Object?>).cast<Map<String, Object?>>();
  final List<Map<String, Object?>> members =
      (report['deadMembers']! as List<Object?>).cast<Map<String, Object?>>();
  final List<Map<String, Object?>> priv = (report['privatizeCandidates']!
          as List<Object?>)
      .cast<Map<String, Object?>>();

  stdout.writeln('scanned ${report['scanned']} Dart files\n');

  if (doFiles) {
    int lines(List<Map<String, Object?>> l) =>
        l.fold(0, (int a, Map<String, Object?> e) => a + (e['lines']! as int));

    stdout.writeln(
      'UNREACHABLE FROM main.dart AND FROM TESTS  '
      '(${orphans.length} files, ${lines(orphans)} lines)',
    );
    for (final Map<String, Object?> e in orphans) {
      stdout.writeln('  ${e['path']}  (${e['lines']} lines)');
    }
    stdout.writeln();

    stdout.writeln(
      'UNREACHABLE FROM main.dart, STILL TESTED  '
      '(${testOnly.length} files, ${lines(testOnly)} lines) '
      '-- delete with the listed tests',
    );
    for (final Map<String, Object?> e in testOnly) {
      stdout.writeln('  ${e['path']}  (${e['lines']} lines)');
      for (final Object? t in (e['tests'] as List<Object?>? ?? const <Object?>[])) {
        stdout.writeln('      $t');
      }
    }
    stdout.writeln();

    stdout.writeln(
      'TEST HELPERS NO SUITE IMPORTS  '
      '(${helpers.length} files, ${lines(helpers)} lines)',
    );
    for (final Map<String, Object?> e in helpers) {
      stdout.writeln('  ${e['path']}  (${e['lines']} lines)');
    }
    stdout.writeln();
  }

  if (doSymbols) {
    stdout.writeln('UNUSED PUBLIC TOP-LEVEL DECLARATIONS (${dead.length})');
    for (final Map<String, Object?> e in dead) {
      stdout.writeln(
        '  ${e['path']}:${e['line']}  ${e['kind']} ${e['name']}',
      );
    }
    stdout.writeln();

    stdout.writeln(
      'UNUSED PUBLIC CLASS MEMBERS (${members.length})',
    );
    for (final Map<String, Object?> e in members) {
      stdout.writeln(
        '  ${e['path']}:${e['line']}  ${e['kind']} ${e['name']}',
      );
    }

    if (!showAdvisory) return;
    stdout.writeln();

    stdout.writeln(
      'USED ONLY INSIDE THEIR OWN LIBRARY -- privatize, advisory '
      '(${priv.length})',
    );
    for (final Map<String, Object?> e in priv) {
      stdout.writeln(
        '  ${e['path']}:${e['line']}  ${e['kind']} ${e['name']}',
      );
    }
  }
}
