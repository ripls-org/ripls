import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';

// Test widget that uses the ContentEditingMixin
class TestWidget extends ConsumerStatefulWidget {
  const TestWidget({super.key});

  @override
  ConsumerState<TestWidget> createState() => TestWidgetState();
}

class TestWidgetState extends ConsumerState<TestWidget>
    with ContentEditingMixin {
  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          Text('Title: $editingTitle'),
          Text('Description: $editingDescription'),
          Text('Initialized: $hasInitializedEditing'),
        ],
      ),
    );
  }
}

void main() {
  group('ContentEditingMixin', () {
    testWidgets('initializes with empty values', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      expect(state.editingTitle, equals(''));
      expect(state.editingDescription, equals(''));
      expect(state.hasInitializedEditing, isFalse);
    });

    testWidgets('initializeEditing sets values correctly', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      state.initializeEditing(
        initialTitle: 'Test Title',
        initialDescription: 'Test Description',
      );

      await tester.pump();

      expect(state.editingTitle, equals('Test Title'));
      expect(state.editingDescription, equals('Test Description'));
      expect(state.hasInitializedEditing, isTrue);

      // Verify UI updated
      expect(find.text('Title: Test Title'), findsOneWidget);
      expect(find.text('Description: Test Description'), findsOneWidget);
      expect(find.text('Initialized: true'), findsOneWidget);
    });

    testWidgets('initializeEditing is idempotent', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      state.initializeEditing(
        initialTitle: 'First Title',
        initialDescription: 'First Description',
      );

      await tester.pump();

      // Try to initialize again with different values
      state.initializeEditing(
        initialTitle: 'Second Title',
        initialDescription: 'Second Description',
      );

      await tester.pump();

      // Should still have first values
      expect(state.editingTitle, equals('First Title'));
      expect(state.editingDescription, equals('First Description'));
      expect(state.hasInitializedEditing, isTrue);
    });

    testWidgets('updateEditingTitle updates title', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      state.updateEditingTitle('New Title');
      await tester.pump();

      expect(state.editingTitle, equals('New Title'));
      expect(find.text('Title: New Title'), findsOneWidget);
    });

    testWidgets('updateEditingDescription updates description',
        (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      state.updateEditingDescription('New Description');
      await tester.pump();

      expect(state.editingDescription, equals('New Description'));
      expect(find.text('Description: New Description'), findsOneWidget);
    });

    testWidgets('resetEditing clears all values', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      // Initialize
      state.initializeEditing(
        initialTitle: 'Test Title',
        initialDescription: 'Test Description',
      );

      await tester.pump();

      // Update
      state.updateEditingTitle('Updated Title');
      await tester.pump();

      // Reset
      state.resetEditing();
      await tester.pump();

      expect(state.editingTitle, equals(''));
      expect(state.editingDescription, equals(''));
      expect(state.hasInitializedEditing, isFalse);

      expect(find.text('Title: '), findsOneWidget);
      expect(find.text('Description: '), findsOneWidget);
      expect(find.text('Initialized: false'), findsOneWidget);
    });

    testWidgets('resetEditing allows reinitialization', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      // Initialize
      state.initializeEditing(
        initialTitle: 'First Title',
        initialDescription: 'First Description',
      );

      await tester.pump();

      // Reset
      state.resetEditing();
      await tester.pump();

      // Initialize again with new values
      state.initializeEditing(
        initialTitle: 'Second Title',
        initialDescription: 'Second Description',
      );

      await tester.pump();

      expect(state.editingTitle, equals('Second Title'));
      expect(state.editingDescription, equals('Second Description'));
      expect(state.hasInitializedEditing, isTrue);
    });

    testWidgets('multiple updates work correctly', (tester) async {
      await tester.pumpWidget(
        const ProviderScope(
          child: MaterialApp(
            localizationsDelegates: [
              AppLocalizations.delegate,
              GlobalMaterialLocalizations.delegate,
              GlobalWidgetsLocalizations.delegate,
              GlobalCupertinoLocalizations.delegate,
            ],
            supportedLocales: AppLocalizations.supportedLocales,
            home: TestWidget(),
          ),
        ),
      );

      final state =
          tester.state<TestWidgetState>(find.byType(TestWidget));

      state.updateEditingTitle('Title 1');
      await tester.pump();
      expect(state.editingTitle, equals('Title 1'));

      state.updateEditingTitle('Title 2');
      await tester.pump();
      expect(state.editingTitle, equals('Title 2'));

      state.updateEditingDescription('Description 1');
      await tester.pump();
      expect(state.editingDescription, equals('Description 1'));

      state.updateEditingDescription('Description 2');
      await tester.pump();
      expect(state.editingDescription, equals('Description 2'));
    });
  });
}
