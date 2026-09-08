import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_footer_buttons.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';

import '../../core/theme/app_colors.dart';
import 'feedback/feedback_sheet.dart';

// ---------------------------------------------------------------------------
// Data model
// ---------------------------------------------------------------------------

enum _Action {
  shareGear,
  inviteExperience,
  requestSomething,
  inviteUser,
}

class _TypeItem {
  final String label;
  final IconData icon;
  final _Action action;
  final bool isOther;

  const _TypeItem({
    required this.label,
    required this.icon,
    required this.action,
    this.isOther = false,
  });
}

class _TypeGroup {
  final String title;
  final List<Color> gradientColors;
  final List<_TypeItem> items;

  const _TypeGroup({
    required this.title,
    required this.gradientColors,
    required this.items,
  });
}

// Per-category gradients are intentional informational accents — they identify
// Ask/Do/Share at a glance and survive the glass migration (#1802 scope rule).
// Do not flatten to a uniform glass color.
const _groups = [
  _TypeGroup(
    title: 'Ask',
    gradientColors: [Color(0xFF8B7355), Color(0xFFA0916F)],
    items: [
      _TypeItem(
        label: 'Help',
        icon: Icons.front_hand_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Babysit',
        icon: Icons.baby_changing_station_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Donation',
        icon: Icons.volunteer_activism_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Pet-sit',
        icon: Icons.pets_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Ride',
        icon: Icons.local_taxi_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Tutor',
        icon: Icons.school_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Repair',
        icon: Icons.handyman_outlined,
        action: _Action.requestSomething,
      ),
      _TypeItem(
        label: 'Other',
        icon: Icons.auto_awesome_outlined,
        action: _Action.requestSomething,
        isOther: true,
      ),
    ],
  ),
  _TypeGroup(
    title: 'Do',
    gradientColors: [Color(0xFF6B8F71), Color(0xFF8AAF8F)],
    items: [
      _TypeItem(
        label: 'Eat',
        icon: Icons.dinner_dining_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Hike',
        icon: Icons.hiking_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Carpool',
        icon: Icons.directions_car_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Adventure',
        icon: Icons.landscape_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Dance',
        icon: Icons.music_note_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Study',
        icon: Icons.auto_stories_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Garden',
        icon: Icons.yard_outlined,
        action: _Action.inviteExperience,
      ),
      _TypeItem(
        label: 'Other',
        icon: Icons.auto_awesome_outlined,
        action: _Action.inviteExperience,
        isOther: true,
      ),
    ],
  ),
  _TypeGroup(
    title: 'Share',
    gradientColors: AppColors.lightGradient,
    items: [
      _TypeItem(
        label: 'Tools',
        icon: Icons.build_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Clothes',
        icon: Icons.checkroom_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Books',
        icon: Icons.menu_book_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Furniture',
        icon: Icons.chair_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Games',
        icon: Icons.sports_esports_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Kids stuff',
        icon: Icons.child_friendly_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Food',
        icon: Icons.restaurant_outlined,
        action: _Action.shareGear,
      ),
      _TypeItem(
        label: 'Other',
        icon: Icons.inventory_2_outlined,
        action: _Action.shareGear,
        isOther: true,
      ),
    ],
  ),
];

// ---------------------------------------------------------------------------
// Widget
// ---------------------------------------------------------------------------

/// Modal that shows category-based type buttons when the user taps the plus button.
///
/// Displays three tabs — Ask, Do, Share — each with a grid of circular type
/// buttons. Tapping any button dismisses the modal via [onClose] and invokes
/// the corresponding action callback. An invite link is shown above the
/// feedback link at the bottom.
class PlusButtonModal extends StatefulWidget {
  final VoidCallback onShareGear;
  final VoidCallback onInviteExperience;
  final VoidCallback onRequestSomething;
  final VoidCallback onInviteUser;
  final VoidCallback onClose;

  const PlusButtonModal({
    super.key,
    required this.onShareGear,
    required this.onInviteExperience,
    required this.onRequestSomething,
    required this.onInviteUser,
    required this.onClose,
  });

  @override
  State<PlusButtonModal> createState() => _PlusButtonModalState();
}

class _PlusButtonModalState extends State<PlusButtonModal>
    with TickerProviderStateMixin {
  int _selectedTab = 0;
  late final AnimationController _gridController;

  @override
  void initState() {
    super.initState();
    _gridController = AnimationController(
      vsync: this,
      duration: Duration.zero,
    );
  }

  bool _gridStarted = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _gridController.duration =
        accessibleDuration(context, const Duration(milliseconds: 600));
    if (!_gridStarted) {
      _gridStarted = true;
      _gridController.forward();
    }
  }

  @override
  void dispose() {
    _gridController.dispose();
    super.dispose();
  }

  void _selectTab(int index) {
    if (_selectedTab == index) return;
    setState(() => _selectedTab = index);
    _gridController.reset();
    _gridController.forward();
  }

  void _handleItemTap(_Action action) {
    HapticFeedback.lightImpact();
    widget.onClose();
    switch (action) {
      case _Action.shareGear:
        widget.onShareGear();
      case _Action.inviteExperience:
        widget.onInviteExperience();
      case _Action.requestSomething:
        widget.onRequestSomething();
      case _Action.inviteUser:
        widget.onInviteUser();
    }
  }

  // Returns a staggered per-item animation (30 ms delay per item).
  Animation<double> _itemAnimation(int index) {
    const totalMs = 600.0;
    const itemMs = 300.0;
    final start = (30.0 * index / totalMs).clamp(0.0, 1.0);
    final end = (30.0 * index / totalMs + itemMs / totalMs).clamp(0.0, 1.0);
    return CurvedAnimation(
      parent: _gridController,
      curve: Interval(start, end, curve: Curves.easeOutCubic),
    );
  }

  @override
  Widget build(BuildContext context) {
    return GlassSheet(
      padding: const EdgeInsets.fromLTRB(22, 4, 22, 16),
      child: SafeArea(
        top: false,
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _buildTitle(context),
              const SizedBox(height: 20),
              _buildSegmentedControl(),
              const SizedBox(height: 20),
              _buildGrid(context),
              const SizedBox(height: 16),
              _buildBottomButtons(context),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildTitle(BuildContext context) {
    return Text(
      "I'd like to...",
      style: Theme.of(context).textTheme.headlineMedium?.copyWith(
        color: AppColors.modalTextPrimary,
        fontSize: 26,
        fontWeight: FontWeight.bold,
        height: 1.1,
      ),
    );
  }

  Widget _buildSegmentedControl() {
    return LayoutBuilder(
      builder: (context, constraints) {
        final tabWidth = constraints.maxWidth / _groups.length;
        final activeGradient = _groups[_selectedTab].gradientColors;

        return Stack(
          children: [
            // Track background.
            Container(
              height: 40,
              decoration: BoxDecoration(
                color: AppColors.modalSearchFieldBackground,
                borderRadius: BorderRadius.circular(10),
                border: Border.all(color: AppColors.modalSearchFieldBorder),
              ),
            ),
            // Animated sliding pill — keeps the per-category gradient as an
            // informational accent (#1802 scope rule).
            AnimatedPositioned(
              duration: accessibleDuration(context, const Duration(milliseconds: 300)),
              curve: Curves.easeOutCubic,
              left: _selectedTab * tabWidth + 4,
              top: 4,
              width: tabWidth - 8,
              height: 32,
              child: AnimatedContainer(
                duration: accessibleDuration(context, const Duration(milliseconds: 300)),
                decoration: BoxDecoration(
                  gradient: LinearGradient(colors: activeGradient),
                  borderRadius: BorderRadius.circular(8),
                ),
              ),
            ),
            // Tab labels rendered above the pill.
            SizedBox(
              height: 40,
              child: Row(
                children: [
                  for (int i = 0; i < _groups.length; i++)
                    Expanded(
                      child: Toggle(
                        semanticsLabel: _groups[i].title,
                        selected: _selectedTab == i,
                        onTap: () => _selectTab(i),
                        child: Center(
                          child: AnimatedDefaultTextStyle(
                            duration: accessibleDuration(context, const Duration(milliseconds: 200)),
                            style: TextStyle(
                              color: _selectedTab == i
                                  ? AppColors.modalTextPrimary
                                  : AppColors.modalTextMuted,
                              fontWeight: _selectedTab == i
                                  ? FontWeight.bold
                                  : FontWeight.normal,
                              fontSize: 14,
                            ),
                            child: Text(_groups[i].title),
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            ),
          ],
        );
      },
    );
  }

  Widget _buildGrid(BuildContext context) {
    final items = _groups[_selectedTab].items;
    final gradientColors = _groups[_selectedTab].gradientColors;

    return GridView.count(
      crossAxisCount: 4,
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      crossAxisSpacing: 0,
      mainAxisSpacing: 10,
      childAspectRatio: 0.8,
      children: List.generate(items.length, (i) {
        final anim = _itemAnimation(i);
        return AnimatedBuilder(
          animation: anim,
          builder: (context, child) {
            final value = anim.value;
            return Opacity(
              opacity: value,
              child: Transform.translate(
                offset: Offset(0, 10 * (1 - value)),
                child: child,
              ),
            );
          },
          child: _buildTypeButton(context, items[i], gradientColors),
        );
      }),
    );
  }

  Widget _buildTypeButton(
    BuildContext context,
    _TypeItem item,
    List<Color> gradientColors,
  ) {
    return Tappable(
      semanticsLabel: item.label,
      onTap: () => _handleItemTap(item.action),
      excludeChildSemantics: false,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          SizedBox(
            width: 54,
            height: 54,
            // The per-category gradient is preserved as a glyph accent on each
            // button — it identifies the category at a glance. The translucent
            // surrounding circle reads as glass; the gradient stays vivid.
            child: item.isOther
                ? CustomPaint(
                    painter: _DashedCirclePainter(
                      color: gradientColors.first.withValues(alpha: 0.6),
                    ),
                    child: Center(
                      child: Icon(
                        item.icon,
                        size: 22,
                        color: AppColors.modalTextMuted,
                      ),
                    ),
                  )
                : Container(
                    decoration: BoxDecoration(
                      color: AppColors.modalSearchFieldBackground,
                      shape: BoxShape.circle,
                      border: Border.all(
                        color: AppColors.modalSearchFieldBorder,
                      ),
                    ),
                    child: Center(
                      child: Icon(
                        item.icon,
                        size: 22,
                        color: gradientColors.first,
                      ),
                    ),
                  ),
          ),
          const SizedBox(height: 6),
          Text(
            item.label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w500,
              color: item.isOther
                  ? AppColors.modalTextMuted
                  : AppColors.modalTextPrimary,
              height: 1.2,
            ),
            textAlign: TextAlign.center,
            maxLines: 2,
            overflow: TextOverflow.ellipsis,
          ),
        ],
      ),
    );
  }

  Widget _buildBottomButtons(BuildContext context) {
    final l10n = context.l10n;
    return GlassFooterButtons(
      secondaryLabel: l10n.sidebarFeedback,
      secondaryEnabled: true,
      onSecondary: () {
        Navigator.pop(context);
        if (!context.mounted) return;
        FeedbackSheet.show(context);
      },
      primaryLabel: l10n.plusButtonInviteSomeone,
      primaryEnabled: true,
      onPrimary: () {
        HapticFeedback.lightImpact();
        widget.onClose();
        widget.onInviteUser();
      },
    );
  }
}

// ---------------------------------------------------------------------------
// Painters
// ---------------------------------------------------------------------------

/// Paints a dashed circle border for "Other" type buttons.
class _DashedCirclePainter extends CustomPainter {
  final Color color;

  const _DashedCirclePainter({required this.color});

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = color
      ..strokeWidth = 1.5
      ..style = PaintingStyle.stroke;

    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2 - 1;

    // Draw 12 evenly spaced dashes around the circle.
    const dashCount = 12;
    const sweepPerDash = (2 * pi) / (dashCount * 2);

    for (int i = 0; i < dashCount; i++) {
      final startAngle = i * 2 * sweepPerDash;
      canvas.drawArc(
        Rect.fromCircle(center: center, radius: radius),
        startAngle,
        sweepPerDash,
        false,
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(_DashedCirclePainter old) => old.color != color;
}
