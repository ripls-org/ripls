import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/data/gen/ripls/api/request.pb.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart';
import 'package:ripls/presentation/viewmodels/request_state.dart' as vm;

/// Unit coverage for the Manage-overflow gating getters on the request view
/// state — the predicates that drive the always-on top-right `···` and which
/// rows the Manage sheet shows. Kept ViewModel-first per docs/client/testing.md
/// so the gating is asserted without a widget tree.
void main() {
  const ownerId = 'owner-1';
  const otherId = 'other-9';

  vm.RequestState stateFor(RequestState protoState, {required String viewer}) {
    return vm.RequestState(
      currentUserId: viewer,
      requestDetails: Request(
        id: 'r1',
        state: protoState,
        requester: User(id: ownerId, name: 'Owner'),
      ),
    );
  }

  group('showManageOverflow', () {
    test('owner sees it while active', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_ACTIVE, viewer: ownerId)
            .showManageOverflow,
        isTrue,
      );
    });

    test('owner sees it when fulfilled (for View Impact)', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_FULFILLED, viewer: ownerId)
            .showManageOverflow,
        isTrue,
      );
    });

    test('owner does not see it when cancelled', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_CANCELLED, viewer: ownerId)
            .showManageOverflow,
        isFalse,
      );
    });

    test('non-owner never sees it', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_ACTIVE, viewer: otherId)
            .showManageOverflow,
        isFalse,
      );
      expect(
        stateFor(RequestState.REQUEST_STATE_FULFILLED, viewer: otherId)
            .showManageOverflow,
        isFalse,
      );
    });
  });

  group('showManageSettingsActions', () {
    test('owner gets edit/fulfill/close while active', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_ACTIVE, viewer: ownerId)
            .showManageSettingsActions,
        isTrue,
      );
    });

    test('settings actions hidden in terminal states', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_FULFILLED, viewer: ownerId)
            .showManageSettingsActions,
        isFalse,
      );
      expect(
        stateFor(RequestState.REQUEST_STATE_CANCELLED, viewer: ownerId)
            .showManageSettingsActions,
        isFalse,
      );
    });

    test('non-owner never gets settings actions', () {
      expect(
        stateFor(RequestState.REQUEST_STATE_ACTIVE, viewer: otherId)
            .showManageSettingsActions,
        isFalse,
      );
    });
  });
}
