import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/presentation/screens/create/unified_create_input_drawer.dart'
    show UnifiedPillTabs, UnifiedPillTabEntry;
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';

/// Two-segment Lend / Give toggle for the unified-create preview card.
/// Visible only when the inferred type is Item (gear). Defaults to
/// Lend; user-controlled, not server-classified. Uses the shared
/// [UnifiedPillTabs] primitive so deselected text matches the
/// item/event/request type selector's deselected style.
///
/// See `docs/design/unified-create.md` § Lend / Give toggle.
class UnifiedLendGiveToggle extends ConsumerWidget {
  const UnifiedLendGiveToggle({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(unifiedCreateViewModelProvider);
    if (state.type != DetectedContentType.DETECTED_CONTENT_TYPE_GEAR) {
      return const SizedBox.shrink();
    }
    final vm = ref.read(unifiedCreateViewModelProvider.notifier);
    return UnifiedPillTabs<TransferIntent>(
      value: state.transferIntent,
      entries: [
        UnifiedPillTabEntry(
          value: TransferIntent.lend,
          label: context.l10n.unifiedCreateLend,
        ),
        UnifiedPillTabEntry(
          value: TransferIntent.give,
          label: context.l10n.unifiedCreateGive,
        ),
      ],
      onChange: vm.setTransferIntent,
    );
  }
}
