import 'dart:async';

import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/semantic_announcer.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// Coaching strip rendered over the live camera preview on the
/// unified-create modal. Teaches first-time users what kinds of
/// subjects the camera is good at capturing by cycling through four
/// example thumbnails (gear, event flyer, clothes, food) on a 3-second
/// timer with a nudge sentence that changes per active example.
///
/// Tapping a thumbnail locks the active example and cancels the
/// auto-cycle. The nudge text is wrapped in [LiveRegion] so VoiceOver
/// and TalkBack re-announce the new sentence each time it changes.
///
/// Reads all strings from [AppLocalizations] via `context.l10n`; the
/// only colour primitives used are [AppColors.modalTextPrimary] and the
/// on-glass border tokens — no `Colors.white` literals.
class CameraCoachingCarousel extends StatefulWidget {
  const CameraCoachingCarousel({super.key});

  @override
  State<CameraCoachingCarousel> createState() => _CameraCoachingCarouselState();
}

/// Internal identifier for a coaching example. Used to drive both the
/// rendered thumbnail placeholder (icon + colour) and the
/// l10n-resolved nudge / a11y label.
enum _CoachingExample { gear, flyer, clothes, food }

class _CameraCoachingCarouselState extends State<CameraCoachingCarousel> {
  static const _examples = _CoachingExample.values;
  static const _cycleInterval = Duration(seconds: 3);

  int _currentIndex = 0;
  bool _userLocked = false;
  Timer? _cycleTimer;

  @override
  void initState() {
    super.initState();
    _cycleTimer = Timer.periodic(_cycleInterval, (_) {
      if (!mounted || _userLocked) return;
      setState(() {
        _currentIndex = (_currentIndex + 1) % _examples.length;
      });
    });
  }

  @override
  void dispose() {
    _cycleTimer?.cancel();
    super.dispose();
  }

  void _onPick(int index) {
    if (!mounted) return;
    setState(() {
      _userLocked = true;
      _currentIndex = index;
    });
    _cycleTimer?.cancel();
  }

  String _nudgeFor(_CoachingExample e) {
    final l = context.l10n;
    switch (e) {
      case _CoachingExample.gear:
        return l.unifiedCreateCameraCoachingGear;
      case _CoachingExample.clothes:
        return l.unifiedCreateCameraCoachingClothes;
      case _CoachingExample.food:
        return l.unifiedCreateCameraCoachingFood;
      case _CoachingExample.flyer:
        return l.unifiedCreateCameraCoachingFlyer;
    }
  }

  String _semanticsLabelFor(_CoachingExample e) {
    final l = context.l10n;
    switch (e) {
      case _CoachingExample.gear:
        return l.a11yUnifiedCreateCoachingGear;
      case _CoachingExample.clothes:
        return l.a11yUnifiedCreateCoachingClothes;
      case _CoachingExample.food:
        return l.a11yUnifiedCreateCoachingFood;
      case _CoachingExample.flyer:
        return l.a11yUnifiedCreateCoachingFlyer;
    }
  }

  String _assetPathFor(_CoachingExample e) {
    switch (e) {
      case _CoachingExample.gear:
        return 'assets/images/gear.jpg';
      case _CoachingExample.clothes:
        return 'assets/images/clothes.jpg';
      case _CoachingExample.food:
        return 'assets/images/food.jpg';
      case _CoachingExample.flyer:
        return 'assets/images/flyer.jpg';
    }
  }

  @override
  Widget build(BuildContext context) {
    final activeExample = _examples[_currentIndex];
    final nudge = _nudgeFor(activeExample);
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        // Thumbnail row.
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          mainAxisSize: MainAxisSize.min,
          children: [
            for (int i = 0; i < _examples.length; i++) ...[
              _CoachingThumbnail(
                example: _examples[i],
                active: i == _currentIndex,
                semanticsLabel: _semanticsLabelFor(_examples[i]),
                assetPath: _assetPathFor(_examples[i]),
                onTap: () => _onPick(i),
              ),
              if (i < _examples.length - 1) const SizedBox(width: 10),
            ],
          ],
        ),
        const SizedBox(height: 10),
        // Cycling nudge text. LiveRegion so VoiceOver / TalkBack
        // re-announces on every change. The AnimatedSwitcher fades the
        // new sentence in; key on the example so the switcher knows it
        // changed.
        LiveRegion(
          child: AnimatedSwitcher(
            duration: accessibleDuration(
                context, const Duration(milliseconds: 300)),
            // Sequential, so two nudges are never painted over each other
            // mid-rotation — see home_empty_states (#2784).
            switchOutCurve: const Interval(0.5, 1),
            switchInCurve: const Interval(0.5, 1),
            child: Text(
              nudge,
              key: ValueKey(activeExample),
              textAlign: TextAlign.center,
              style: TextStyle(
                color: AppColors.modalTextPrimary,
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                shadows: const [
                  Shadow(
                    color: Color(0x80000000),
                    blurRadius: 4,
                    offset: Offset(0, 1),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}

class _CoachingThumbnail extends StatelessWidget {
  const _CoachingThumbnail({
    required this.example,
    required this.active,
    required this.semanticsLabel,
    required this.assetPath,
    required this.onTap,
  });

  final _CoachingExample example;
  final bool active;
  final String semanticsLabel;
  final String assetPath;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final activeSize = 64.0;
    final inactiveSize = 50.0;
    // The border lives on an outer ring so the inner image fills the
    // rounded rectangle without leaving a translucent gap.
    final outerRadius = BorderRadius.circular(12);
    final innerRadius = BorderRadius.circular(10);
    return Tappable(
      key: Key('camera-coaching-thumb-${example.name}'),
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: AnimatedContainer(
        duration:
            accessibleDuration(context, const Duration(milliseconds: 500)),
        curve: Curves.easeOut,
        width: active ? activeSize : inactiveSize,
        height: active ? activeSize : inactiveSize,
        decoration: BoxDecoration(
          borderRadius: outerRadius,
          color: AppColors.modalSurface,
          border: Border.all(
            color: active ? GlassTokens.borderActive : GlassTokens.border,
            width: active ? 2 : 1,
          ),
          boxShadow: active
              ? const [
                  BoxShadow(
                    color: Color(0x40000000),
                    blurRadius: 8,
                    offset: Offset(0, 2),
                  ),
                ]
              : null,
        ),
        alignment: Alignment.center,
        clipBehavior: Clip.antiAlias,
        child: AnimatedOpacity(
          duration:
              accessibleDuration(context, const Duration(milliseconds: 500)),
          opacity: active ? 1.0 : 0.55,
          child: ClipRRect(
            borderRadius: innerRadius,
            child: Image.asset(
              assetPath,
              fit: BoxFit.cover,
              width: double.infinity,
              height: double.infinity,
              // Render a transparent placeholder when the asset can't
              // be loaded (e.g. widget tests without an asset bundle).
              // The thumbnail's outer container still shows the
              // translucent glass surface + border, so the carousel
              // stays usable as a tap target.
              errorBuilder: (_, _, _) => const SizedBox.expand(),
            ),
          ),
        ),
      ),
    );
  }
}
