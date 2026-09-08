import 'package:fixnum/fixnum.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/time.pb.dart'
    show ExperienceTime, SpecificTime;
import 'package:ripls/presentation/providers/user_timezone_provider.dart';
import 'package:ripls/presentation/widgets/experience/propose_time_form.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';

/// PreviewTimePickerSheet is a bottom sheet time picker that uses
/// [ProposeTimeForm] for date/time/duration selection.
///
/// Used in the experience preview modal where no server experience ID exists
/// yet. Returns an [ExperienceTime] via [Navigator.pop] when the user taps
/// the primary footer action.
///
/// Renders on the modal-glass surface so the picker matches the rest of
/// the app's date/time pickers — the form, calendar/time/duration
/// dialogs, and Set-Time CTA all use the same material.
class PreviewTimePickerSheet extends ConsumerStatefulWidget {
  const PreviewTimePickerSheet({super.key, this.initialTime});

  final ExperienceTime? initialTime;

  @override
  ConsumerState<PreviewTimePickerSheet> createState() =>
      _PreviewTimePickerSheetState();
}

class _PreviewTimePickerSheetState
    extends ConsumerState<PreviewTimePickerSheet> {
  DateTime? _date;
  TimeOfDay? _time;
  int? _duration;

  @override
  void initState() {
    super.initState();
    final initial = widget.initialTime;
    if (initial != null && initial.hasSpecific()) {
      final dt = DateTime.fromMillisecondsSinceEpoch(
        initial.specific.unixTimestampSec.toInt() * 1000,
      );
      _date = dt;
      _time = TimeOfDay(hour: dt.hour, minute: dt.minute);
      _duration = initial.specific.durationMinutes;
    }
  }

  @override
  Widget build(BuildContext context) {
    final canSet = _date != null && _time != null && _duration != null;

    return GlassSheet(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Flexible(
            child: SingleChildScrollView(
              child: ProposeTimeForm(
                selectedDate: _date,
                selectedTime: _time,
                selectedDuration: _duration,
                existingDate: _date,
                existingTime: _time,
                existingDuration: _duration,
                onDateChanged: (d) => setState(() => _date = d),
                onTimeChanged: (t) => setState(() => _time = t),
                onDurationChanged: (dur) => setState(() => _duration = dur),
                allowPastDates: true,
              ),
            ),
          ),
          GlassFooterButtons(
            primaryLabel: 'Set Time',
            primaryEnabled: canSet,
            onPrimary: canSet ? _submit : null,
            onSecondary: () => Navigator.of(context).pop(),
          ),
        ],
      ),
    );
  }

  Future<void> _submit() async {
    final date = _date;
    final time = _time;
    final duration = _duration;
    if (date == null || time == null || duration == null) return;

    final dateTime = DateTime(
      date.year,
      date.month,
      date.day,
      time.hour,
      time.minute,
    );
    final unixSec = dateTime.millisecondsSinceEpoch ~/ 1000;
    final userTz = await ref.read(resolvedTimezoneProvider.future);

    if (!mounted) return;
    Navigator.of(context).pop(
      ExperienceTime(
        specific: SpecificTime(
          unixTimestampSec: Int64(unixSec),
          timezone: userTz,
          durationMinutes: duration,
        ),
      ),
    );
  }
}
