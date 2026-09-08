import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/models/discover_item.dart';
import 'package:ripls/presentation/models/magazine_item.dart';
import 'package:ripls/presentation/viewmodels/discover_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/magazine_thumbnail.dart';

/// DiscoverGalleryOverlay is a horizontal carousel of MagazineThumbnail cards
/// shown above the bottom sheet when a map marker is tapped.
///
/// The selected item is centered with adjacent items peeking in from either
/// side. Swiping updates the map selection; tapping navigates to the detail
/// screen.
class DiscoverGalleryOverlay extends ConsumerStatefulWidget {
  final List<DiscoverItem> items;
  final int initialIndex;
  final Function(DiscoverItem, int) onPageChanged;
  final Function(DiscoverItem) onItemTap;
  final Map<String, double?> distanceMap;

  const DiscoverGalleryOverlay({
    super.key,
    required this.items,
    required this.initialIndex,
    required this.onPageChanged,
    required this.onItemTap,
    required this.distanceMap,
  });

  @override
  ConsumerState<DiscoverGalleryOverlay> createState() =>
      _DiscoverGalleryOverlayState();
}

class _DiscoverGalleryOverlayState
    extends ConsumerState<DiscoverGalleryOverlay> {
  late PageController _pageController;
  late int _currentPage;

  static const int _infiniteMultiplier = 1000;

  @override
  void initState() {
    super.initState();
    if (widget.items.isEmpty || widget.items.length == 1) {
      _currentPage = 0;
      _pageController = PageController(
        initialPage: 0,
        viewportFraction: 0.75,
      );
    } else {
      _currentPage = widget.initialIndex.clamp(0, widget.items.length - 1);
      final middleOffset = (_infiniteMultiplier ~/ 2) * widget.items.length;
      _pageController = PageController(
        initialPage: middleOffset + _currentPage,
        viewportFraction: 0.75,
      );
    }
  }

  @override
  void didUpdateWidget(DiscoverGalleryOverlay oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.initialIndex != oldWidget.initialIndex &&
        widget.initialIndex != _currentPage &&
        widget.initialIndex >= 0 &&
        widget.initialIndex < widget.items.length) {
      _currentPage = widget.initialIndex;
      final currentPageInList = _pageController.page?.round() ?? 0;
      final currentActualIndex = currentPageInList % widget.items.length;
      final diff = widget.initialIndex - currentActualIndex;
      _pageController.animateToPage(
        currentPageInList + diff,
        duration: accessibleDuration(context, const Duration(milliseconds: 300)),
        curve: Curves.easeInOut,
      );
    }
  }

  @override
  void dispose() {
    _pageController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (widget.items.isEmpty) return const SizedBox.shrink();

    final discoverState = ref.watch(discoverProvider);

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        _buildPageIndicator(),
        const SizedBox(height: 8),
        SizedBox(
          height: 220,
          child: PageView.builder(
            controller: _pageController,
            onPageChanged: (index) {
              final actualIndex = index % widget.items.length;
              setState(() => _currentPage = actualIndex);
              widget.onPageChanged(widget.items[actualIndex], actualIndex);
            },
            itemCount: widget.items.length == 1
                ? 1
                : widget.items.length * _infiniteMultiplier,
            itemBuilder: (context, index) {
              final actualIndex = index % widget.items.length;
              final item = widget.items[actualIndex];
              final isCurrent = actualIndex == _currentPage;
              final locationName =
                  _locationNameForItem(item, discoverState);

              return AnimatedScale(
                scale: isCurrent ? 1.0 : 0.85,
                duration: accessibleDuration(context, const Duration(milliseconds: 300)),
                curve: Curves.easeOut,
                child: Padding(
                  padding: const EdgeInsets.symmetric(horizontal: 8),
                  child: MagazineThumbnail(
                    item: MagazineItem.fromDiscoverItem(
                      item,
                      locationName: locationName,
                    ),
                    onTap: () => widget.onItemTap(item),
                  ),
                ),
              );
            },
          ),
        ),
      ],
    );
  }

  /// Returns a pre-resolved location name and triggers async resolution when
  /// not yet cached.
  String? _locationNameForItem(DiscoverItem item, DiscoverState state) {
    return item.when(
      gear: (gear) {
        if (gear.locationId.isEmpty) return null;
        Future.microtask(
          () => ref
              .read(discoverProvider.notifier)
              .resolveLocationName(gear.locationId),
        );
        return state.locationNames[gear.locationId];
      },
      request: (req) => null,
      experience: (exp) {
        if (exp.locationId.isEmpty) return null;
        Future.microtask(
          () => ref
              .read(discoverProvider.notifier)
              .resolveLocationName(exp.locationId),
        );
        return state.locationNames[exp.locationId];
      },
    );
  }

  Widget _buildPageIndicator() {
    if (widget.items.length <= 1) return const SizedBox.shrink();

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: AppColors.surface(context).withValues(alpha: 0.9),
        borderRadius: BorderRadius.circular(16),
      ),
      child: Text(
        '${_currentPage + 1} / ${widget.items.length}',
        style: Theme.of(context).textTheme.bodySmall?.copyWith(
              color: AppColors.textPrimary(context),
              fontWeight: FontWeight.w500,
            ),
      ),
    );
  }
}
