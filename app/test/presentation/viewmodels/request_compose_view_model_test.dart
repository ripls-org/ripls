import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/gen/ripls/api/request_service.pb.dart'
    show BatchClaimRequestNeedItem, BatchClaimRequestNeedResult;
import 'package:ripls/data/repositories/request_repository.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/services/providers.dart';

import 'request_compose_view_model_test.mocks.dart';

@GenerateMocks([RequestRepository])
void main() {
  late MockRequestRepository repo;
  late ProviderContainer container;

  setUp(() {
    repo = MockRequestRepository();
    container = ProviderContainer(
      overrides: [
        requestRepositoryProvider.overrideWithValue(repo),
      ],
    );
    addTearDown(container.dispose);
  });

  RequestComposeNotifier readNotifier() =>
      container.read(requestComposeProvider.notifier);
  RequestComposeState readState() => container.read(requestComposeProvider);

  group('chip + piece state', () {
    test('toggleSuggestion adds and removes a piece by chip label', () {
      final n = readNotifier();
      n.toggleSuggestion('Stump grinder');
      expect(readState().pieces.length, 1);
      expect(readState().pieces.first.label, 'Stump grinder');
      expect(readState().pieces.first.fromSuggestionLabel, 'Stump grinder');

      n.toggleSuggestion('Stump grinder');
      expect(readState().pieces, isEmpty);
    });

    test('addPasteList splits on commas, semicolons, and newlines', () {
      readNotifier().addPasteList('Stump grinder, Haul debris;Provide lunch\nTarp');
      final labels = readState().pieces.map((p) => p.label).toList();
      expect(labels, ['Stump grinder', 'Haul debris', 'Provide lunch', 'Tarp']);
    });

    test('addInlinePiece trims and appends', () {
      final n = readNotifier();
      n.addInlinePiece('  Saw  ');
      expect(readState().pieces.single.label, 'Saw');
    });

    test('addInlinePiece ignores empty input', () {
      readNotifier().addInlinePiece('   ');
      expect(readState().pieces, isEmpty);
    });

    test('renamePiece updates the label on commit', () {
      final n = readNotifier();
      n.addInlinePiece('Saw');
      final id = readState().pieces.first.id;
      n.renamePiece(id, 'Chainsaw');
      expect(readState().pieces.first.label, 'Chainsaw');
    });

    test('renamePiece ignores empty rename', () {
      final n = readNotifier();
      n.addInlinePiece('Saw');
      final id = readState().pieces.first.id;
      n.renamePiece(id, '   ');
      expect(readState().pieces.first.label, 'Saw');
    });

    test('togglePreclaim flips the asker self-claim flag', () {
      final n = readNotifier();
      n.addInlinePiece('Lunch');
      final id = readState().pieces.first.id;
      expect(readState().preclaimedCount, 0);
      n.togglePreclaim(id);
      expect(readState().preclaimedCount, 1);
      n.togglePreclaim(id);
      expect(readState().preclaimedCount, 0);
    });

    test('removePiece drops the piece by id', () {
      final n = readNotifier()
        ..addInlinePiece('A')
        ..addInlinePiece('B');
      final firstId = readState().pieces.first.id;
      n.removePiece(firstId);
      expect(readState().pieces.length, 1);
      expect(readState().pieces.single.label, 'B');
    });

    test('canPublish reflects pieces present and not publishing', () {
      expect(readState().canPublish, isFalse);
      readNotifier().addInlinePiece('A');
      expect(readState().canPublish, isTrue);
    });

    test('reset clears all draft state', () {
      readNotifier()
        ..addInlinePiece('A')
        ..addInlinePiece('B');
      expect(readState().pieces.length, 2);
      readNotifier().reset();
      expect(readState().pieces, isEmpty);
    });
  });

  group('publish() happy path', () {
    test('three-step: SubmitRequest is owned by caller, then batch add + pre-claim',
        () async {
      when(repo.addNeedsBatch(
        requestId: anyNamed('requestId'),
        items: anyNamed('items'),
      )).thenAnswer((_) async {
        final n1 = RequestNeedResponse()..id = 'need_1';
        final n2 = RequestNeedResponse()..id = 'need_2';
        return [n1, n2];
      });
      when(repo.batchClaimNeeds(
        requestId: anyNamed('requestId'),
        claims: anyNamed('claims'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async {
        final r = BatchClaimRequestNeedResult()..needId = 'need_2';
        return [r];
      });

      final n = readNotifier()
        ..addInlinePiece('Saw')
        ..addInlinePiece('Lunch');
      // Pre-claim the second piece.
      n.togglePreclaim(readState().pieces[1].id);

      final result = await n.publish(requestId: 'req_xyz');

      expect(result.requestId, 'req_xyz');
      expect(result.addedNeedIds, ['need_1', 'need_2']);
      expect(result.claimedNeedIds, ['need_2']);
      expect(result.failedAdds, isEmpty);
      expect(result.failedClaims, isEmpty);
      expect(readState().isPublishing, isFalse);

      final capturedClaims =
          verify(repo.batchClaimNeeds(
            requestId: 'req_xyz',
            claims: captureAnyNamed('claims'),
            communityId: anyNamed('communityId'),
          )).captured.single as List<BatchClaimRequestNeedItem>;
      expect(capturedClaims.length, 1);
      expect(capturedClaims.first.needId, 'need_2');
    });

    test('no pre-claimed pieces skips the batch claim leg entirely', () async {
      when(repo.addNeedsBatch(
        requestId: anyNamed('requestId'),
        items: anyNamed('items'),
      )).thenAnswer((_) async {
        return [RequestNeedResponse()..id = 'need_a'];
      });

      final n = readNotifier()..addInlinePiece('Saw');
      final result = await n.publish(requestId: 'req_a');

      expect(result.addedNeedIds, ['need_a']);
      expect(result.claimedNeedIds, isEmpty);
      verifyNever(repo.batchClaimNeeds(
        requestId: anyNamed('requestId'),
        claims: anyNamed('claims'),
        communityId: anyNamed('communityId'),
      ));
    });

    test('empty draft returns immediately with no calls', () async {
      final result = await readNotifier().publish(requestId: 'req_empty');
      expect(result.addedNeedIds, isEmpty);
      verifyNever(repo.addNeedsBatch(
        requestId: anyNamed('requestId'),
        items: anyNamed('items'),
      ));
    });
  });

  group('publish() partial success', () {
    test('batch-add throws → failedAdds populated, claim leg not called',
        () async {
      when(repo.addNeedsBatch(
        requestId: anyNamed('requestId'),
        items: anyNamed('items'),
      )).thenThrow(Exception('add failed'));

      final n = readNotifier()
        ..addInlinePiece('Saw')
        ..addInlinePiece('Lunch');
      final result = await n.publish(requestId: 'req_b');

      expect(result.addedNeedIds, isEmpty);
      expect(result.failedAdds.length, 2);
      verifyNever(repo.batchClaimNeeds(
        requestId: anyNamed('requestId'),
        claims: anyNamed('claims'),
        communityId: anyNamed('communityId'),
      ));
      expect(readState().error, isNotNull);
    });

    test('per-claim failure surfaces in failedClaims, success in claimedNeedIds',
        () async {
      when(repo.addNeedsBatch(
        requestId: anyNamed('requestId'),
        items: anyNamed('items'),
      )).thenAnswer((_) async {
        return [
          RequestNeedResponse()..id = 'need_1',
          RequestNeedResponse()..id = 'need_2',
        ];
      });
      when(repo.batchClaimNeeds(
        requestId: anyNamed('requestId'),
        claims: anyNamed('claims'),
        communityId: anyNamed('communityId'),
      )).thenAnswer((_) async => [
            BatchClaimRequestNeedResult()..needId = 'need_1',
            BatchClaimRequestNeedResult()
              ..needId = 'need_2'
              ..errorMessage = 'no slots',
          ]);

      final n = readNotifier()
        ..addInlinePiece('A')
        ..addInlinePiece('B');
      // Pre-claim both.
      for (final p in readState().pieces) {
        n.togglePreclaim(p.id);
      }

      final result = await n.publish(requestId: 'req_c');
      expect(result.claimedNeedIds, ['need_1']);
      expect(result.failedClaims.length, 1);
      expect(result.failedClaims.first.needId, 'need_2');
      expect(result.failedClaims.first.errorMessage, 'no slots');
    });
  });

  group('disposal safety', () {
    test('container.dispose during in-flight publish completes without throwing',
        () async {
      final completer = Completer<List<RequestNeedResponse>>();
      when(repo.addNeedsBatch(
        requestId: anyNamed('requestId'),
        items: anyNamed('items'),
      )).thenAnswer((_) => completer.future);

      readNotifier().addInlinePiece('Saw');
      final future = readNotifier().publish(requestId: 'req_d');

      container.dispose();
      completer.complete([RequestNeedResponse()..id = 'need_late']);

      await expectLater(future, completes);
    });
  });
}
