import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';

/// ContentTab describes a single tab entry in [ContentTabBar].
class ContentTab {
  /// Label text displayed on the tab button.
  final String label;

  /// Optional badge string displayed in a pill above the tab.
  /// For Chat: unread count; for Media: total media count.
  final String? badge;

  /// When true the badge uses the accent background color (dot style).
  /// Used for the Chat unread badge. Media count uses a glass pill instead.
  final bool showBadgeDot;

  /// Optional override color for this tab's active pill. Falls back to the
  /// [ContentTabBar.accentColor] when null.
  final Color? color;

  const ContentTab({
    required this.label,
    this.badge,
    this.showBadgeDot = false,
    this.color,
  });
}

/// ContentTabBar displays a horizontal row of individually-sized pill buttons.
///
/// A single accent-colored background pill slides between the active tab
/// positions (like a bottom navigation indicator). Inactive pills use a glass
/// morphism style. The accent fill animates via [AnimatedPositioned].
class ContentTabBar extends StatefulWidget {
  final List<ContentTab> tabs;
  final int activeIndex;
  final ValueChanged<int> onChanged;
  final Color accentColor;

  /// When true, the Chat badge is hidden (editing mode suppresses it per spec).
  final bool isEditing;

  const ContentTabBar({
    super.key,
    required this.tabs,
    required this.activeIndex,
    required this.onChanged,
    required this.accentColor,
    this.isEditing = false,
  });

  @override
  State<ContentTabBar> createState() => _ContentTabBarState();
}

class _ContentTabBarState extends State<ContentTabBar> {
  // Key for the Stack itself so pill positions are measured relative to it,
  // not the outer Padding widget (which would add a 16px offset).
  final GlobalKey _stackKey = GlobalKey();
  late List<GlobalKey> _keys;
  double _pillLeft = 0;
  double _pillWidth = 0;
  bool _measured = false;

  // Suppress animation on the initial layout measurement so the pill snaps
  // into place rather than sliding from (0, 0).
  bool _animate = false;

  @override
  void initState() {
    super.initState();
    _keys = List.generate(widget.tabs.length, (_) => GlobalKey());
    WidgetsBinding.instance.addPostFrameCallback((_) => _measure(false));
  }

  @override
  void didUpdateWidget(ContentTabBar old) {
    super.didUpdateWidget(old);
    if (old.tabs.length != widget.tabs.length) {
      _keys = List.generate(widget.tabs.length, (_) => GlobalKey());
    }
    // Animate when the active tab changes; re-measure without animation for
    // structural changes (badge text, tab count).
    WidgetsBinding.instance.addPostFrameCallback(
      (_) => _measure(old.activeIndex != widget.activeIndex),
    );
  }

  void _measure(bool animate) {
    if (!mounted) return;
    final idx = widget.activeIndex.clamp(0, _keys.length - 1);
    final box = _keys[idx].currentContext?.findRenderObject() as RenderBox?;
    if (box == null) return;
    // Measure relative to the Stack, not the outer Padding, so that
    // AnimatedPositioned coordinates match the Stack's coordinate space.
    final stackBox =
        _stackKey.currentContext?.findRenderObject() as RenderBox?;
    if (stackBox == null) return;
    final offset = box.localToGlobal(Offset.zero, ancestor: stackBox);
    setState(() {
      _pillLeft = offset.dx;
      _pillWidth = box.size.width;
      _measured = true;
      _animate = animate;
    });
  }

  @override
  Widget build(BuildContext context) {
    // Use the first tab's color override when tab 0 is active; fall back to
    // accentColor for all other tabs so the pill stays a consistent color
    // when switching between Details and Chat.
    final activePillColor = widget.activeIndex == 0
        ? (widget.tabs[0].color ?? widget.accentColor)
        : widget.accentColor;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Stack(
        key: _stackKey,
        clipBehavior: Clip.none,
        children: [
          // Sliding accent background — positioned behind the pill labels.
          if (_measured)
            AnimatedPositioned(
              duration:
                  _animate ? const Duration(milliseconds: 280) : Duration.zero,
              curve: Curves.easeInOut,
              left: _pillLeft,
              top: 0,
              bottom: 0,
              width: _pillWidth,
              child: AnimatedContainer(
                duration: accessibleDuration(context, const Duration(milliseconds: 280)),
                curve: Curves.easeInOut,
                decoration: BoxDecoration(
                  color: activePillColor,
                  borderRadius: BorderRadius.circular(20),
                ),
              ),
            ),
          // Tab pills row — defines the Stack's intrinsic size.
          // Badge pills in each _TabPill use Stack(clipBehavior: Clip.none) and
          // paint outside their bounds without contributing to layout width, so
          // no unconstrained wrapper is needed here.
          Row(
            mainAxisAlignment: MainAxisAlignment.start,
            mainAxisSize: MainAxisSize.max,
            children: [
              for (int i = 0; i < widget.tabs.length; i++) ...[
                if (i > 0) const SizedBox(width: 8),
                _TabPill(
                  key: _keys[i],
                  tab: widget.tabs[i],
                  isActive: i == widget.activeIndex,
                  accentColor: widget.tabs[i].color ?? widget.accentColor,
                  isEditing: widget.isEditing,
                  onTap: () => widget.onChanged(i),
                ),
              ],
            ],
          ),
        ],
      ),
    );
  }
}

class _TabPill extends StatelessWidget {
  final ContentTab tab;
  final bool isActive;
  final Color accentColor;
  final bool isEditing;
  final VoidCallback onTap;

  const _TabPill({
    super.key,
    required this.tab,
    required this.isActive,
    required this.accentColor,
    required this.isEditing,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final showBadge = tab.badge != null && !(isEditing && tab.showBadgeDot);

    // Active pills use an opaque accentColor background so the selection color
    // renders correctly regardless of compositing. Inactive pills use glass.
    final pill = AnimatedContainer(
      duration: accessibleDuration(context, const Duration(milliseconds: 200)),
      curve: Curves.easeInOut,
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 7),
      decoration: isActive
          ? BoxDecoration(
              color: accentColor,
              borderRadius: const BorderRadius.all(Radius.circular(20)),
            )
          : BoxDecoration(
              color: OverlayTokens.fieldFill,
              borderRadius: BorderRadius.circular(20),
              border: Border.all(
                color: GlassTokens.hairline,
              ),
            ),
      child: AnimatedDefaultTextStyle(
        duration: accessibleDuration(context, const Duration(milliseconds: 200)),
        curve: Curves.easeInOut,
        style: TextStyle(
          fontSize: 12,
          fontWeight: isActive ? FontWeight.w700 : FontWeight.w500,
          color: isActive ? GlassTokens.textPrimary : GlassTokens.textMuted,
        ),
        child: Text(tab.label),
      ),
    );

    return Toggle(
      semanticsLabel: tab.label,
      selected: isActive,
      onTap: onTap,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          // Inactive tabs get a glass blur behind the pill.
          if (!isActive)
            ClipRRect(
              borderRadius: BorderRadius.circular(20),
              child: BackdropFilter(
                filter: ImageFilter.blur(sigmaX: 12, sigmaY: 12),
                child: pill,
              ),
            )
          else
            pill,
          if (showBadge)
            Positioned(
              top: -5,
              right: 4,
              child: _BadgePill(
                text: tab.badge!,
                isDot: tab.showBadgeDot,
                accentColor: accentColor,
              ),
            ),
        ],
      ),
    );
  }
}

class _BadgePill extends StatelessWidget {
  final String text;
  final bool isDot;
  final Color accentColor;

  const _BadgePill({
    required this.text,
    required this.isDot,
    required this.accentColor,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
      constraints: const BoxConstraints(minWidth: 16),
      decoration: BoxDecoration(
        color: isDot ? accentColor : AppColors.lightSurface,
        borderRadius: BorderRadius.circular(10),
        border: isDot
            ? null
            : Border.all(color: AppColors.lightTextTertiary, width: 1),
        boxShadow: isDot
            ? [
                BoxShadow(
                  color: Colors.black.withValues(alpha: 0.45),
                  spreadRadius: 2,
                  blurRadius: 0,
                ),
              ]
            : null,
      ),
      child: Text(
        text,
        textAlign: TextAlign.center,
        style: TextStyle(
          color: isDot ? GlassTokens.textPrimary : AppColors.lightTextPrimary,
          fontSize: 9,
          fontWeight: FontWeight.w700,
          height: 16 / 9,
        ),
      ),
    );
  }
}
