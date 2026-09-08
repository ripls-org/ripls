import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_when_tool_button.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// ExperienceWhenDock is the bottom dock of the event "When" screen
/// (event-when-agenda design): a hero (event date/time + an export icon, or an
/// "Exploring … Back to your event" header when the viewer has tapped another
/// day), the day's [weatherStrip] and [agenda], and a host control bar with
/// Mark done · Change time · Ask the group.
///
/// Presentation-only: the panel supplies the resolved strings, the built strip
/// and agenda widgets, and the action callbacks.
class ExperienceWhenDock extends StatelessWidget {
  final bool exploring;

  /// Event hero (when not exploring).
  final String eventDateText;
  final String eventTimeText;
  final bool completed;
  final VoidCallback onExport;

  /// Exploring hero (when exploring another day).
  final String exploringDateText;
  final VoidCallback onBackToEvent;

  final Widget weatherStrip;
  final Widget agenda;

  /// Host control bar — shown only to organizers.
  final bool showControls;
  final VoidCallback? onMarkDone;
  final VoidCallback onChangeTime;
  final VoidCallback onAskGroup;

  const ExperienceWhenDock({
    super.key,
    required this.exploring,
    required this.eventDateText,
    required this.eventTimeText,
    required this.completed,
    required this.onExport,
    required this.exploringDateText,
    required this.onBackToEvent,
    required this.weatherStrip,
    required this.agenda,
    required this.showControls,
    required this.onMarkDone,
    required this.onChangeTime,
    required this.onAskGroup,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        exploring ? _exploringHero(context) : _eventHero(context),
        const SizedBox(height: 16),
        weatherStrip,
        const SizedBox(height: 16),
        agenda,
        if (showControls) ...[
          const SizedBox(height: 16),
          _controls(context),
        ],
      ],
    );
  }

  Widget _eventHero(BuildContext context) {
    final l10n = context.l10n;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                eventDateText,
                style: const TextStyle(
                  color: AppColors.onContentImage,
                  fontSize: 25,
                  fontWeight: FontWeight.w800,
                  height: 1.05,
                ),
              ),
              if (eventTimeText.isNotEmpty) ...[
                const SizedBox(height: 3),
                Text(
                  eventTimeText,
                  style: const TextStyle(
                    color: AppColors.darkTextSecondary,
                    fontSize: 15,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
              if (completed) ...[
                const SizedBox(height: 7),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.check_circle,
                        size: 14, color: AppColors.experienceSageGreen),
                    const SizedBox(width: 5),
                    Text(
                      l10n.timePanelDone.toUpperCase(),
                      style: const TextStyle(
                        color: AppColors.experienceSageGreen,
                        fontSize: 11,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 0.4,
                      ),
                    ),
                  ],
                ),
              ],
            ],
          ),
        ),
        const SizedBox(width: 12),
        Column(
          children: [
            Tappable(
              semanticsLabel: l10n.timePollFinalizedAddToCalendar,
              onTap: onExport,
              inkBorderRadius: BorderRadius.circular(14),
              child: Container(
                width: 46,
                height: 46,
                decoration: BoxDecoration(
                  color: AppColors.experienceSageGreen,
                  borderRadius: BorderRadius.circular(14),
                ),
                child: const Icon(Icons.ios_share,
                    color: AppColors.onContentImage, size: 22),
              ),
            ),
            const SizedBox(height: 3),
            Text(
              l10n.timePanelExport,
              style: const TextStyle(
                color: AppColors.darkTextTertiary,
                fontSize: 9,
                fontWeight: FontWeight.w700,
              ),
            ),
          ],
        ),
      ],
    );
  }

  Widget _exploringHero(BuildContext context) {
    final l10n = context.l10n;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.end,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                l10n.timePanelExploring.toUpperCase(),
                style: const TextStyle(
                  color: AppColors.statusWarningOnDark,
                  fontSize: 10,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 1,
                ),
              ),
              const SizedBox(height: 3),
              Text(
                exploringDateText,
                style: const TextStyle(
                  color: AppColors.onContentImage,
                  fontSize: 23,
                  fontWeight: FontWeight.w800,
                  height: 1.05,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(width: 10),
        Tappable(
          semanticsLabel: l10n.timePanelBackToEvent,
          onTap: onBackToEvent,
          child: Padding(
            padding: const EdgeInsets.only(bottom: 3),
            child: Text(
              l10n.timePanelBackToEvent,
              style: const TextStyle(
                color: AppColors.experienceSageGreenSoftText,
                fontSize: 13,
                fontWeight: FontWeight.w700,
              ),
            ),
          ),
        ),
      ],
    );
  }

  Widget _controls(BuildContext context) {
    final l10n = context.l10n;
    return Container(
      padding: const EdgeInsets.only(top: 13),
      decoration: const BoxDecoration(
        border: Border(
          top: BorderSide(color: GlassTokens.hairline),
        ),
      ),
      child: Row(
        children: [
          Expanded(
            child: WhenToolButton(
              icon: completed ? Icons.check_circle : Icons.check,
              label: completed ? l10n.timePanelDone : l10n.timePanelMarkDone,
              active: completed,
              onTap: onMarkDone,
            ),
          ),
          Expanded(
            child: WhenToolButton(
              icon: Icons.schedule_outlined,
              label: l10n.a11yExpChangeTime,
              onTap: onChangeTime,
            ),
          ),
          Expanded(
            child: WhenToolButton(
              icon: Icons.chat_bubble_outline,
              label: l10n.locationPanelAskGroupShort,
              onTap: onAskGroup,
            ),
          ),
        ],
      ),
    );
  }
}
