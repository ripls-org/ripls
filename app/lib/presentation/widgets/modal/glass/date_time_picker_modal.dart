import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart' hide TextDirection;
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_state.dart';
import 'package:ripls/presentation/viewmodels/date_time_picker_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/modal/glass/date_time_picker_modal_atoms.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_footer_buttons.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';

/// Combined calendar + time-wheel picker — surface-agnostic.
///
/// The canonical replacement for any flow that previously chained
/// `showDatePicker` + `showTimePicker` (or their glass equivalents). The
/// design and rationale live in [docs/issues/2124-combined-datetime-modal.md].
///
/// Renders its own content (no sheet chrome) so it can be embedded inline on a
/// screen *or* wrapped in a bottom sheet via [DateTimePickerModal.show].
/// Reports the user's choice through callbacks ([onSaved] / [onCancel] /
/// [onRemove]) rather than `Navigator.pop`, so inline hosts can act without a
/// route.
class DateTimePickerView extends ConsumerStatefulWidget {
  final DateTime initialDateTime;
  final DateTime firstDate;
  final DateTime lastDate;
  final bool allowRemove;

  /// Called with the chosen [DateTime] when the user taps the primary button.
  final ValueChanged<DateTime> onSaved;

  /// Called when the user cancels (the secondary button, unless [allowRemove]).
  final VoidCallback onCancel;

  /// Called for the destructive-clear action — only wired when [allowRemove].
  final VoidCallback? onRemove;

  /// Label for the green primary button. Defaults to "Save"; callers adding a
  /// candidate (e.g. a poll time) pass "Add" so the verb matches the action.
  final String? primaryLabel;

  const DateTimePickerView({
    super.key,
    required this.initialDateTime,
    required this.firstDate,
    required this.lastDate,
    required this.onSaved,
    required this.onCancel,
    this.onRemove,
    this.allowRemove = false,
    this.primaryLabel,
  });

  @override
  ConsumerState<DateTimePickerView> createState() =>
      _DateTimePickerViewState();
}

/// Bottom-sheet entry point for [DateTimePickerView]. Returns the user's choice
/// through the Navigator result pattern as a [DateTimePickerResult]: `saved`
/// carries the picked `DateTime`, `removed` is the destructive-clear action
/// (only when [allowRemove] is true), and `null` represents a cancel. Inline
/// hosts embed [DateTimePickerView] directly instead.
class DateTimePickerModal {
  const DateTimePickerModal._();

  /// Opens the picker as a glass bottom sheet and returns the user's choice.
  ///
  /// Pass `allowRemove: true` only when the caller has an existing value to
  /// clear — otherwise the Remove button is hidden and `removed` will never
  /// be returned. `initialDateTime` is clamped into `[firstDate, lastDate]`.
  static Future<DateTimePickerResult?> show(
    BuildContext context, {
    required DateTime initialDateTime,
    required DateTime firstDate,
    required DateTime lastDate,
    bool allowRemove = false,
  }) {
    return showAccessibleModal<DateTimePickerResult>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      useRootNavigator: true,
      builder: (sheetContext) => GlassSheet(
        padding: EdgeInsets.fromLTRB(
            20, 20, 20, 28 + MediaQuery.of(sheetContext).viewInsets.bottom),
        child: DateTimePickerView(
          initialDateTime: initialDateTime,
          firstDate: firstDate,
          lastDate: lastDate,
          allowRemove: allowRemove,
          onSaved: (dt) =>
              Navigator.of(sheetContext).pop(DateTimePickerResult.saved(dt)),
          onCancel: () => Navigator.of(sheetContext).pop(),
          onRemove: allowRemove
              ? () => Navigator.of(sheetContext)
                  .pop(const DateTimePickerResult.removed())
              : null,
        ),
      ),
    );
  }
}

class _DateTimePickerViewState extends ConsumerState<DateTimePickerView> {
  bool _initialized = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      ref.read(dateTimePickerProvider.notifier).initialize(
            initial: widget.initialDateTime,
            firstDate: widget.firstDate,
            lastDate: widget.lastDate,
          );
      setState(() => _initialized = true);
    });
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    if (!_initialized) {
      return const SizedBox(
        height: 460,
        child: Center(
          child: CircularProgressIndicator(color: AppColors.modalTextPrimary),
        ),
      );
    }
    final state = ref.watch(dateTimePickerProvider);
    final notifier = ref.read(dateTimePickerProvider.notifier);
    final selected = state.selectedDateTime;

    // Content only — the host supplies the surface (GlassSheet for the modal,
    // the panel scrim for the inline embed).
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _SelectionHero(
          dayLong: DateFormat('EEE, MMM d').format(selected),
          time: _formatHeroTime(selected),
        ),
        const SizedBox(height: 18),
        // Body scrolls on smaller phones — the calendar + wheels can
        // otherwise exceed the available height.
        Flexible(
          child: SingleChildScrollView(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                CalendarGrid(
                  visibleMonth: state.visibleMonth,
                  selectedDate: state.selectedDate,
                  firstDate: state.firstDate,
                  lastDate: state.lastDate,
                  canGoToPreviousMonth: state.canGoToPreviousMonth,
                  canGoToNextMonth: state.canGoToNextMonth,
                  onDaySelected: notifier.selectDay,
                  onPreviousMonth: () => notifier.shiftMonth(-1),
                  onNextMonth: () => notifier.shiftMonth(1),
                ),
                const SizedBox(height: 12),
                TimeWheelGroup(
                  hour12: state.hour12,
                  minute: state.minute,
                  isPm: state.isPm,
                  onHourChanged: notifier.setHour12,
                  onMinuteChanged: notifier.setMinute,
                  onIsPmChanged: notifier.setIsPm,
                ),
              ],
            ),
          ),
        ),
        GlassFooterButtons(
          primaryLabel: widget.primaryLabel ?? l10n.commonSave,
          primaryEnabled: true,
          onPrimary: () => widget.onSaved(selected),
          secondaryLabel:
              widget.allowRemove ? l10n.commonRemove : l10n.commonCancel,
          showSecondary: true,
          onSecondary: widget.allowRemove ? widget.onRemove : widget.onCancel,
        ),
      ],
    );
  }
}

/// The sage-tinted hero card at the top of the modal — large date, sage time,
/// and an optional caller-supplied tz/context footnote. Wraps in a
/// `LiveRegion` so screen readers re-announce as the user spins the wheels
/// or picks a different day.
class _SelectionHero extends StatelessWidget {
  final String dayLong;
  final String time;

  const _SelectionHero({
    required this.dayLong,
    required this.time,
  });

  @override
  Widget build(BuildContext context) {
    return LiveRegion(
      child: Semantics(
        header: true,
        child: Container(
          padding: const EdgeInsets.fromLTRB(18, 16, 18, 16),
          decoration: BoxDecoration(
            color: AppColors.modalPrimaryButtonBackground
                .withValues(alpha: 0.22),
            borderRadius: BorderRadius.circular(18),
            border: Border.all(
              color: AppColors.modalPrimaryButtonBackground
                  .withValues(alpha: 0.55),
              width: 1.5,
            ),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                dayLong,
                style: const TextStyle(
                  fontSize: 26,
                  fontWeight: FontWeight.w800,
                  height: 1.1,
                  letterSpacing: -0.4,
                  color: AppColors.modalTextPrimary,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                time,
                style: const TextStyle(
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.1,
                  color: AppColors.modalTextPrimary,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Formats the time line for the hero — e.g. "12:00 PM PT". Falls back to
/// the platform short zone name from [DateTime.timeZoneName] (e.g. "MDT"
/// on a US-Mountain machine).
String _formatHeroTime(DateTime dt) {
  final time = DateFormat('h:mm a').format(dt);
  return '$time ${dt.timeZoneName}';
}
