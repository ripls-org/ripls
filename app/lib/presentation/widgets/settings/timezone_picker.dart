import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:timezone/timezone.dart' as tz;

/// TimezonePicker displays a searchable list of IANA timezones.
///
/// Shows all available timezones from the timezone package with search
/// functionality. Formats timezone names for better readability.
///
/// When the user has a manual override set, a "Use system timezone" option
/// appears at the top of the list to reset to the device default.
class TimezonePicker extends StatefulWidget {
  /// Currently selected timezone (IANA format, e.g., "America/New_York"),
  /// or null when using the system default.
  final String? currentTimezone;

  /// Callback when timezone is selected. Passes the IANA string, or null
  /// to reset to the system default.
  final ValueChanged<String?> onTimezoneSelected;

  const TimezonePicker({
    super.key,
    this.currentTimezone,
    required this.onTimezoneSelected,
  });

  @override
  State<TimezonePicker> createState() => _TimezonePickerState();
}

class _TimezonePickerState extends State<TimezonePicker> {
  late List<String> _allTimezones;
  late List<String> _filteredTimezones;
  final TextEditingController _searchController = TextEditingController();

  @override
  void initState() {
    super.initState();
    _allTimezones = tz.timeZoneDatabase.locations.keys.toList()..sort();
    _filteredTimezones = _allTimezones;
    _searchController.addListener(_filterTimezones);
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  void _filterTimezones() {
    final query = _searchController.text.toLowerCase();
    setState(() {
      if (query.isEmpty) {
        _filteredTimezones = _allTimezones;
      } else {
        _filteredTimezones = _allTimezones
            .where((tz) => tz.toLowerCase().contains(query))
            .toList();
      }
    });
  }

  String _formatTimezoneDisplay(String timezone) {
    return timezone.replaceAll('_', ' ');
  }

  @override
  Widget build(BuildContext context) {
    final systemTz = tz.local.name;
    final hasManualOverride = widget.currentTimezone != null;
    final isSearching = _searchController.text.isNotEmpty;

    return Scaffold(
      appBar: AppBar(
        title: Text(context.l10n.settingsTimezone),
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(56),
          child: Padding(
            padding: const EdgeInsets.all(8),
            child: TextField(
              controller: _searchController,
              decoration: InputDecoration(
                hintText: context.l10n.settingsTimezoneSearchHint,
                prefixIcon: const Icon(Icons.search),
                suffixIcon: _searchController.text.isNotEmpty
                    ? IconAction(
                        icon: Icons.clear,
                        semanticsLabel: context.l10n.a11yMiscClearSearch,
                        onPressed: () {
                          _searchController.clear();
                        },
                      )
                    : null,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(8),
                ),
                filled: true,
              ),
            ),
          ),
        ),
      ),
      body: ListView.builder(
        itemCount: _filteredTimezones.length +
            (hasManualOverride && !isSearching ? 1 : 0),
        itemBuilder: (context, index) {
          // "Use system timezone" reset option at top when override is set
          if (hasManualOverride && !isSearching && index == 0) {
            return Column(
              children: [
                ListTile(
                  leading: Icon(
                    Icons.phone_android,
                    color: AppColors.textSecondary(context),
                  ),
                  title: Text(context.l10n.settingsTimezoneUseSystem),
                  subtitle: Text(
                    _formatTimezoneDisplay(systemTz),
                    style: TextStyle(color: AppColors.textSecondary(context)),
                  ),
                  onTap: () {
                    widget.onTimezoneSelected(null);
                    Navigator.pop(context);
                  },
                ),
                Divider(height: 1, color: AppColors.border(context)),
              ],
            );
          }

          final tzIndex = hasManualOverride && !isSearching ? index - 1 : index;
          final timezone = _filteredTimezones[tzIndex];
          final isSelected = timezone == widget.currentTimezone;
          // When no manual override, highlight the system timezone
          final isSystemDefault =
              widget.currentTimezone == null && timezone == systemTz;

          return ListTile(
            title: Text(_formatTimezoneDisplay(timezone)),
            trailing: isSelected
                ? const Icon(Icons.check)
                : isSystemDefault
                    ? Text(
                        context.l10n.settingsTimezoneDefault,
                        style: TextStyle(
                          fontSize: 12,
                          color: AppColors.textTertiary(context),
                        ),
                      )
                    : null,
            selected: isSelected,
            onTap: () {
              widget.onTimezoneSelected(timezone);
              Navigator.pop(context);
            },
          );
        },
      ),
    );
  }
}
