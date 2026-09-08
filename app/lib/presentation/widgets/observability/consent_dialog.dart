import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/observability/settings.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/widgets/modal/modal_header.dart';
import 'package:ripls/presentation/widgets/modal/modal_helpers.dart';
import 'package:ripls/services/providers.dart';

/// ObservabilityConsentDialog displays a modal for users to opt-in or opt-out
/// of observability features (crash reporting, performance monitoring, analytics).
class ObservabilityConsentDialog extends ConsumerStatefulWidget {
  /// Whether this is the initial consent prompt (shows different UI than settings)
  final bool isInitialPrompt;

  const ObservabilityConsentDialog({
    super.key,
    this.isInitialPrompt = true,
  });

  /// Show the consent dialog as a modal bottom sheet.
  ///
  /// Returns true if user made consent decisions, false if dismissed.
  static Future<bool?> show(
    BuildContext context, {
    bool isInitialPrompt = true,
  }) {
    return ModalHelpers.showStandardModal<bool>(
      context,
      builder: (context) => ObservabilityConsentDialog(
        isInitialPrompt: isInitialPrompt,
      ),
      heightFactor: 0.70,
    );
  }

  @override
  ConsumerState<ObservabilityConsentDialog> createState() =>
      _ObservabilityConsentDialogState();
}

class _ObservabilityConsentDialogState
    extends ConsumerState<ObservabilityConsentDialog> {
  late bool _crashReportingEnabled;
  late bool _performanceEnabled;
  late bool _analyticsEnabled;
  bool _isSaving = false;

  @override
  void initState() {
    super.initState();
    final settings = ref.read(observabilitySettingsProvider);
    _crashReportingEnabled = settings.crashReporting == ObservabilityConsent.granted;
    _performanceEnabled = settings.performanceMonitoring == ObservabilityConsent.granted;
    _analyticsEnabled = settings.analytics == ObservabilityConsent.granted;
  }

  Future<void> _saveSettings() async {
    setState(() => _isSaving = true);
    final navigator = Navigator.of(context);

    try {
      await ref.read(observabilitySettingsProvider.notifier).setAll(
        crashReporting: _crashReportingEnabled
            ? ObservabilityConsent.granted
            : ObservabilityConsent.denied,
        performanceMonitoring: _performanceEnabled
            ? ObservabilityConsent.granted
            : ObservabilityConsent.denied,
        analytics: _analyticsEnabled
            ? ObservabilityConsent.granted
            : ObservabilityConsent.denied,
      );

      if (mounted) {
        navigator.pop(true);
      }
    } finally {
      if (mounted) {
        setState(() => _isSaving = false);
      }
    }
  }

  Future<void> _acceptAll() async {
    setState(() {
      _crashReportingEnabled = true;
      _performanceEnabled = true;
      _analyticsEnabled = true;
    });
    await _saveSettings();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: AppColors.background(context),
      body: SafeArea(
        child: Column(
          children: [
            _buildDragHandle(),
            ModalHeader(
              title: widget.isInitialPrompt ? 'Data Consent' : 'Privacy & Data',
            ),
            Expanded(
              child: _buildContent(context),
            ),
            _buildFooter(context),
          ],
        ),
      ),
    );
  }

  Widget _buildDragHandle() {
    return Container(
      padding: const EdgeInsets.symmetric(vertical: 12),
      child: Center(
        child: Container(
          width: 40,
          height: 4,
          decoration: BoxDecoration(
            color: AppColors.border(context),
            borderRadius: BorderRadius.circular(2),
          ),
        ),
      ),
    );
  }

  Widget _buildContent(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Container(
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: AppColors.border(context),
            width: 1,
          ),
        ),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            _buildToggleItem(
              context,
              icon: Icons.bug_report_outlined,
              title: 'Crash Reporting',
              subtitle: 'Help us fix bugs by sharing crash reports',
              value: _crashReportingEnabled,
              onChanged: (value) => setState(() => _crashReportingEnabled = value),
            ),
            _buildDivider(context),
            _buildToggleItem(
              context,
              icon: Icons.speed_outlined,
              title: 'Performance Monitoring',
              subtitle: 'Share performance data to improve app speed',
              value: _performanceEnabled,
              onChanged: (value) => setState(() => _performanceEnabled = value),
            ),
            _buildDivider(context),
            _buildToggleItem(
              context,
              icon: Icons.analytics_outlined,
              title: 'Usage Analytics',
              subtitle: 'Share anonymous usage data to improve Ripls',
              value: _analyticsEnabled,
              onChanged: (value) => setState(() => _analyticsEnabled = value),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildToggleItem(
    BuildContext context, {
    required IconData icon,
    required String title,
    required String subtitle,
    required bool value,
    required ValueChanged<bool> onChanged,
  }) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: AppColors.primary(context).withValues(alpha: 0.1),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Icon(
              icon,
              color: AppColors.primary(context),
              size: 20,
            ),
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                        fontWeight: FontWeight.w500,
                        color: AppColors.textPrimary(context),
                      ),
                ),
                const SizedBox(height: 2),
                Text(
                  subtitle,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                        color: AppColors.textSecondary(context),
                      ),
                ),
              ],
            ),
          ),
          Switch.adaptive(
            value: value,
            onChanged: onChanged,
            activeTrackColor: AppColors.primary(context),
            activeThumbColor: Colors.white,
          ),
        ],
      ),
    );
  }

  Widget _buildDivider(BuildContext context) {
    return Divider(
      height: 1,
      thickness: 1,
      indent: 56,
      color: AppColors.border(context),
    );
  }

  Widget _buildFooter(BuildContext context) {
    return Container(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 16),
      child: SafeArea(
        top: false,
        child: widget.isInitialPrompt
            ? _buildInitialFooter(context)
            : _buildSettingsFooter(context),
      ),
    );
  }

  Widget _buildInitialFooter(BuildContext context) {
    return Row(
      children: [
        Expanded(
          child: ElevatedButton(
            onPressed: _isSaving ? null : _acceptAll,
            style: ElevatedButton.styleFrom(
              backgroundColor: AppColors.primary(context),
              foregroundColor: AppColors.onPrimary(context),
              padding: const EdgeInsets.symmetric(vertical: 12),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(50),
              ),
              elevation: 0,
            ),
            child: _isSaving
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      valueColor: AlwaysStoppedAnimation(Colors.white),
                    ),
                  )
                : const Text(
                    'Accept All',
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
          ),
        ),
        const SizedBox(width: 12),
        Expanded(
          child: OutlinedButton(
            onPressed: _isSaving ? null : _saveSettings,
            style: OutlinedButton.styleFrom(
              foregroundColor: AppColors.textSecondary(context),
              padding: const EdgeInsets.symmetric(vertical: 12),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(50),
              ),
              side: BorderSide(
                color: AppColors.border(context),
                width: 1.5,
              ),
            ),
            child: const Text(
              'Save',
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _buildSettingsFooter(BuildContext context) {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton(
        onPressed: _isSaving ? null : _saveSettings,
        style: ElevatedButton.styleFrom(
          backgroundColor: AppColors.primary(context),
          foregroundColor: AppColors.onPrimary(context),
          padding: const EdgeInsets.symmetric(vertical: 12),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(50),
          ),
          elevation: 0,
        ),
        child: _isSaving
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  valueColor: AlwaysStoppedAnimation(Colors.white),
                ),
              )
            : const Text(
                'Save',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                ),
              ),
      ),
    );
  }
}
