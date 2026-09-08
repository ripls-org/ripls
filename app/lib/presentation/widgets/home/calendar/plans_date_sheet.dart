import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:intl/intl.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// A named community offered in the sheet's "Show groups" scope chips.
class PlansSheetGroup {
  const PlansSheetGroup({required this.id, required this.name});
  final String id;
  final String name;
}

/// The Plans tab's date & filter bottom sheet (liquid-glass mock, Library
/// location-sheet grammar). Deliberately **not** a second month grid —
/// paging months is what the main view is for. Instead "Go to date" is a
/// typed M / D / Y entry with the weekday resolved live beneath it, so a
/// far-off date is two seconds away instead of a dozen swipes. Quick
/// chips cover the common jumps; "Show groups" chips scope which plans
/// appear.
///
/// There is no confirm button: every change applies to the calendar
/// behind the sheet **immediately** — a valid typed date or quick chip
/// fires [onDateChanged], a group chip fires [onGroupChanged]. Dismissing
/// the sheet is just closing it.
class PlansDateSheet extends StatefulWidget {
  const PlansDateSheet({
    super.key,
    required this.initialDate,
    required this.groups,
    required this.onDateChanged,
    required this.onGroupChanged,
    this.selectedGroupId,
  });

  final DateTime initialDate;
  final List<PlansSheetGroup> groups;

  /// The currently active community scope, or null for all groups.
  final String? selectedGroupId;

  /// Fired whenever the typed/chipped values resolve to a valid date.
  final ValueChanged<DateTime> onDateChanged;

  /// Fired when the community scope changes (null = all groups).
  final ValueChanged<String?> onGroupChanged;

  @override
  State<PlansDateSheet> createState() => _PlansDateSheetState();
}

class _PlansDateSheetState extends State<PlansDateSheet> {
  late final TextEditingController _month;
  late final TextEditingController _day;
  late final TextEditingController _year;
  final _monthFocus = FocusNode();
  final _dayFocus = FocusNode();
  final _yearFocus = FocusNode();
  String? _groupId;

  @override
  void initState() {
    super.initState();
    _month = TextEditingController(
        text: widget.initialDate.month.toString().padLeft(2, '0'));
    _day = TextEditingController(
        text: widget.initialDate.day.toString().padLeft(2, '0'));
    _year = TextEditingController(text: widget.initialDate.year.toString());
    _groupId = widget.selectedGroupId;
  }

  @override
  void dispose() {
    _month.dispose();
    _day.dispose();
    _year.dispose();
    _monthFocus.dispose();
    _dayFocus.dispose();
    _yearFocus.dispose();
    super.dispose();
  }

  /// The typed values as a real calendar date, or null while they don't
  /// form one (e.g. 02/31). The DateTime round-trip catches overflow.
  DateTime? get _resolved {
    final m = int.tryParse(_month.text);
    final d = int.tryParse(_day.text);
    final y = int.tryParse(_year.text);
    if (m == null || d == null || y == null) return null;
    if (y < 2000 || y > 2100 || m < 1 || m > 12 || d < 1) return null;
    final date = DateTime(y, m, d);
    if (date.month != m || date.day != d) return null;
    return date;
  }

  void _setDate(DateTime d) {
    setState(() {
      _month.text = d.month.toString().padLeft(2, '0');
      _day.text = d.day.toString().padLeft(2, '0');
      _year.text = d.year.toString();
    });
    widget.onDateChanged(d);
  }

  /// Live-applies the typed values the moment they form a real date.
  void _onTyped() {
    setState(() {});
    final resolved = _resolved;
    if (resolved != null) widget.onDateChanged(resolved);
  }

  void _selectGroup(String? id) {
    setState(() => _groupId = id);
    widget.onGroupChanged(id);
  }

  DateTime get _today {
    final now = DateTime.now();
    return DateTime(now.year, now.month, now.day);
  }

  /// The coming Saturday — or today when the weekend is already here.
  DateTime get _thisWeekend {
    final today = _today;
    if (today.weekday >= DateTime.saturday) return today;
    return today.add(Duration(days: DateTime.saturday - today.weekday));
  }

  /// The Monday strictly after today.
  DateTime get _nextWeek {
    final today = _today;
    return today.add(Duration(days: 8 - today.weekday));
  }

  DateTime get _nextMonth => DateTime(_today.year, _today.month + 1, 1);

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final locale = Localizations.localeOf(context).toString();
    final resolved = _resolved;

    return Container(
      padding: EdgeInsets.fromLTRB(
          20, 12, 20, 30 + MediaQuery.of(context).viewInsets.bottom),
      decoration: BoxDecoration(
        color: AppColors.background(context),
        borderRadius: const BorderRadius.vertical(top: Radius.circular(30)),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Center(
            child: Container(
              width: 38,
              height: 4.5,
              margin: const EdgeInsets.only(bottom: 14),
              decoration: BoxDecoration(
                color: AppColors.textTertiary(context).withValues(alpha: 0.5),
                borderRadius: BorderRadius.circular(3),
              ),
            ),
          ),
          Text(
            l10n.plansSheetTitle,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 20,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary(context),
            ),
          ),
          const SizedBox(height: 9),
          Row(
            children: [
              Expanded(
                child: _segment(context,
                    controller: _month,
                    focus: _monthFocus,
                    next: _dayFocus,
                    label: l10n.plansSheetMonth,
                    maxLength: 2),
              ),
              _separator(context),
              Expanded(
                child: _segment(context,
                    controller: _day,
                    focus: _dayFocus,
                    next: _yearFocus,
                    label: l10n.plansSheetDay,
                    maxLength: 2),
              ),
              _separator(context),
              Expanded(
                flex: 2,
                child: _segment(context,
                    controller: _year,
                    focus: _yearFocus,
                    next: null,
                    label: l10n.plansSheetYear,
                    maxLength: 4),
              ),
            ],
          ),
          const SizedBox(height: 9),
          Text(
            resolved != null
                ? DateFormat.yMMMMEEEEd(locale).format(resolved)
                : l10n.plansSheetInvalidDate,
            style: TextStyle(
              fontSize: 12.5,
              color: resolved != null
                  ? AppColors.textSecondary(context)
                  : AppColors.statusError(context),
            ),
          ),
          const SizedBox(height: 11),
          Wrap(
            spacing: 6,
            runSpacing: 6,
            children: [
              _chip(context, l10n.plansChipToday, false,
                  () => _setDate(_today)),
              _chip(context, l10n.plansChipThisWeekend, false,
                  () => _setDate(_thisWeekend)),
              _chip(context, l10n.plansChipNextWeek, false,
                  () => _setDate(_nextWeek)),
              _chip(context, l10n.plansChipNextMonth, false,
                  () => _setDate(_nextMonth)),
            ],
          ),
          if (widget.groups.isNotEmpty) ...[
            _sectionHeader(context, l10n.plansSheetShowGroups),
            Wrap(
              spacing: 6,
              runSpacing: 6,
              children: [
                _chip(context, l10n.plansChipAllGroups, _groupId == null,
                    () => _selectGroup(null)),
                for (final g in widget.groups)
                  _chip(context, g.name, _groupId == g.id,
                      () => _selectGroup(g.id)),
              ],
            ),
          ],
          const SizedBox(height: 8),
        ],
      ),
    );
  }

  /// One typed segment of the M / D / Y entry: a number field styled as
  /// the mock's dseg box, its uppercase caption below. Filling a segment
  /// advances focus to [next].
  Widget _segment(
    BuildContext context, {
    required TextEditingController controller,
    required FocusNode focus,
    required FocusNode? next,
    required String label,
    required int maxLength,
  }) {
    return Container(
      padding: const EdgeInsets.fromLTRB(4, 8, 4, 8),
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: AppColors.border(context)),
      ),
      child: Column(
        children: [
          TextField(
            controller: controller,
            focusNode: focus,
            keyboardType: TextInputType.number,
            inputFormatters: [
              FilteringTextInputFormatter.digitsOnly,
              LengthLimitingTextInputFormatter(maxLength),
            ],
            textAlign: TextAlign.center,
            style: TextStyle(
              fontFamily: AppTheme.headingFont,
              fontSize: 24,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary(context),
            ),
            decoration: const InputDecoration(
              isDense: true,
              border: InputBorder.none,
              contentPadding: EdgeInsets.zero,
              counterText: '',
            ),
            onChanged: (v) {
              if (v.length >= maxLength && next != null) {
                next.requestFocus();
              }
              _onTyped();
            },
          ),
          const SizedBox(height: 4),
          Text(
            label.toUpperCase(),
            style: TextStyle(
              fontSize: 9,
              fontWeight: FontWeight.w700,
              letterSpacing: 1.1,
              color: AppColors.textTertiary(context),
            ),
          ),
        ],
      ),
    );
  }

  Widget _separator(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 6),
      child: Center(
        child: Text(
          '/',
          style: TextStyle(
            fontSize: 22,
            fontWeight: FontWeight.w300,
            color: AppColors.textTertiary(context),
          ),
        ),
      ),
    );
  }

  Widget _sectionHeader(BuildContext context, String text) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(2, 14, 2, 6),
      child: Text(
        text.toUpperCase(),
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.6,
          color: AppColors.textSecondary(context),
        ),
      ),
    );
  }

  Widget _chip(
      BuildContext context, String label, bool on, VoidCallback onTap) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 13, vertical: 7),
        decoration: BoxDecoration(
          color: on
              ? AppColors.primary(context)
              : AppColors.cardBackground(context),
          borderRadius: BorderRadius.circular(999),
          border: Border.all(
            color: on ? Colors.transparent : AppColors.border(context),
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 12,
            fontWeight: on ? FontWeight.w600 : FontWeight.w500,
            color: on
                ? AppColors.background(context)
                : AppColors.textSecondary(context),
          ),
        ),
      ),
    );
  }

}
