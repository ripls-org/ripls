import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_chat_pane.dart';
import 'package:ripls/presentation/screens/experience/widgets/experience_read_shell.dart';
import 'package:ripls/presentation/viewmodels/experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/back_button.dart';

/// ExperienceConversationCarousel carousels between the read shell and the
/// full-screen discussion *without* tearing down the hero
/// (docs/issues/2278-experience-content-redesign.md). Both panes share the
/// content view's persistent `VideoBackgroundHost` behind this widget, so
/// opening the conversation slides the read shell off to the left and the chat
/// in from the right with no interruption to the background video.
///
/// "Open" is driven entirely by the view-model: the conversation is showing
/// whenever `activeTab == chatTabIndex` (and not editing). The read shell's tap
/// opens it via [onOpenConversation]; the in-pane back button and the Android
/// back gesture close it by resetting the active tab to 0. This keeps a single
/// source of truth that the host screen can also read (to hide its own back
/// button while the conversation is up).
///
/// A horizontal drag also drives the carousel directly: swipe left to pull the
/// conversation in, swipe right to push it back. The drag scrubs the controller
/// in real time and settles to the nearest end (or follows a fling) on release,
/// committing the result back to the active tab. The feed swipes vertically and
/// the chat scrolls vertically, so the horizontal drag never fights them.
/// Decides whether a horizontal carousel drag should end with the conversation
/// open. A fling past [flingVelocity] commits in its direction (negative px/s =
/// leftward = open); otherwise the drag settles to whichever end [position]
/// (0 closed … 1 open) is nearer. Pure and exposed for testing.
@visibleForTesting
bool resolveCarouselDragTarget({
  required double velocity,
  required double position,
  double flingVelocity = 400,
}) {
  if (velocity <= -flingVelocity) return true;
  if (velocity >= flingVelocity) return false;
  return position >= 0.5;
}

class ExperienceConversationCarousel extends ConsumerStatefulWidget {
  final String experienceId;
  final Color accentColor;

  /// Opens the conversation by sliding it in (the carousel's own swipe-commit).
  /// The host wires this to set the active tab to chat (and hide the feed nav
  /// when embedded).
  final VoidCallback onOpenConversation;

  /// Expands the discussion-card tap into the full conversation, morphing from
  /// the card's footprint over the hero (docs/client/modals.md).
  final ValueChanged<Rect> onExpandConversation;

  /// Expands the time ("WHEN") card into the full-screen time panel, morphing
  /// from the card's footprint over the hero (docs/client/modals.md).
  final ValueChanged<Rect> onShowTime;

  /// Expands the location ("WHERE") card into the full-screen location panel,
  /// morphing from the card's footprint over the hero (docs/client/modals.md).
  final ValueChanged<Rect> onShowLocation;
  final VoidCallback onShowAccess;
  final VoidCallback onManage;
  final double bottomNavInset;

  /// Chat media callbacks, threaded through to the discussion pane.
  final Future<void> Function() onAddMedia;
  final Future<void> Function(String mediaId) onDeleteMedia;
  final Future<void> Function(List<String> mediaIds)? onReorderMedia;

  const ExperienceConversationCarousel({
    super.key,
    required this.experienceId,
    required this.accentColor,
    required this.onOpenConversation,
    required this.onExpandConversation,
    required this.onShowTime,
    required this.onShowLocation,
    required this.onShowAccess,
    required this.onManage,
    required this.onAddMedia,
    required this.onDeleteMedia,
    this.onReorderMedia,
    this.bottomNavInset = 0,
  });

  @override
  ConsumerState<ExperienceConversationCarousel> createState() =>
      _ExperienceConversationCarouselState();
}

class _ExperienceConversationCarouselState
    extends ConsumerState<ExperienceConversationCarousel>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  /// True while a horizontal drag is in flight, so the conversation pane stays
  /// mounted even before the active tab flips to chat.
  bool _dragging = false;

  /// Fling velocity (px/s) past which a drag commits to open/close regardless
  /// of how far it travelled.
  static const double _flingVelocity = 400;

  @override
  void initState() {
    super.initState();
    // The duration is set (via accessibleDuration) in _animate before every
    // forward/reverse, so it honours the system "Reduce Motion" setting.
    _controller = AnimationController(vsync: this);
    // Unmount the chat pane once the close animation fully settles.
    _controller.addStatusListener((status) {
      if (status == AnimationStatus.dismissed && mounted) setState(() {});
    });
    // Sync to the initial state (e.g. a deep link that lands on chat) without
    // animating.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!mounted) return;
      _controller.value = _isOpen(ref.read(experienceProvider(widget.experienceId)))
          ? 1
          : 0;
    });
    // Animate the carousel whenever the open/closed state flips. Registered
    // here (not via ref.listen in build) so it isn't re-subscribed each frame.
    ref.listenManual(
      experienceProvider(widget.experienceId).select(_isOpen),
      (_, next) => _animate(next),
    );
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  bool _isOpen(ExperienceState state) =>
      !state.isEditing && state.activeTab == state.chatTabIndex;

  void _close() {
    ref
        .read(experienceProvider(widget.experienceId).notifier)
        .setActiveTab(0);
  }

  void _animate(bool open) {
    _controller.duration = accessibleDuration(
      context,
      const Duration(milliseconds: 300),
    );
    if (open) {
      _controller.forward();
    } else {
      _controller.reverse();
    }
  }

  void _onDragStart(DragStartDetails details) {
    if (!_dragging) setState(() => _dragging = true);
  }

  void _onDragUpdate(DragUpdateDetails details) {
    final width = MediaQuery.of(context).size.width;
    if (width <= 0) return;
    // Drag left (negative dx) pulls the conversation in (value → 1); drag right
    // pushes it back out (value → 0).
    _controller.value = (_controller.value - details.primaryDelta! / width)
        .clamp(0.0, 1.0);
  }

  void _onDragEnd(DragEndDetails details) {
    final open = resolveCarouselDragTarget(
      velocity: details.primaryVelocity ?? 0,
      position: _controller.value,
      flingVelocity: _flingVelocity,
    );
    if (mounted) setState(() => _dragging = false);
    _settle(open);
  }

  /// Commits the drag result. Flipping the active tab lets the `ref.listen`
  /// below animate the controller from wherever the drag left it; when the state
  /// is unchanged (a drag that didn't cross the threshold) the controller is
  /// re-settled directly.
  void _settle(bool open) {
    final currentlyOpen = _isOpen(
      ref.read(experienceProvider(widget.experienceId)),
    );
    if (open == currentlyOpen) {
      _animate(open);
    } else if (open) {
      widget.onOpenConversation();
    } else {
      _close();
    }
  }

  @override
  Widget build(BuildContext context) {
    final open = ref.watch(
      experienceProvider(widget.experienceId).select(_isOpen),
    );

    final width = MediaQuery.of(context).size.width;
    final readShell = ExperienceReadShell(
      experienceId: widget.experienceId,
      accentColor: widget.accentColor,
      onExpandConversation: widget.onExpandConversation,
      onShowTime: widget.onShowTime,
      onShowLocation: widget.onShowLocation,
      onShowAccess: widget.onShowAccess,
      onManage: widget.onManage,
      bottomNavInset: widget.bottomNavInset,
    );
    // Build the conversation pane whenever it is (or is still animating, or
    // being dragged) on screen, so there is something to slide in/out.
    final conversation = (open || _dragging || _controller.value > 0.0)
        ? _conversation(context)
        : null;

    return PopScope(
      canPop: !open,
      onPopInvokedWithResult: (didPop, _) {
        if (!didPop) _close();
      },
      child: GestureDetector(
        behavior: HitTestBehavior.translucent,
        onHorizontalDragStart: _onDragStart,
        onHorizontalDragUpdate: _onDragUpdate,
        onHorizontalDragEnd: _onDragEnd,
        child: AnimatedBuilder(
          animation: _controller,
          builder: (context, _) {
            final v = Curves.easeInOut.transform(_controller.value);
            return Stack(
              fit: StackFit.expand,
              children: [
                Transform.translate(
                  offset: Offset(-width * v, 0),
                  child: IgnorePointer(ignoring: v > 0.001, child: readShell),
                ),
                if (conversation != null)
                  Transform.translate(
                    offset: Offset(width * (1 - v), 0),
                    child: conversation,
                  ),
              ],
            );
          },
        ),
      ),
    );
  }

  /// The discussion foreground: a uniform scrim over the shared hero so the
  /// messages stay legible, the chat pane, a top fade so transcript text
  /// dissolves before reaching the controls (#2724), and a back button
  /// that closes the carousel (the host screen hides its own while this is
  /// up, so there is a single dismiss affordance).
  Widget _conversation(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        Positioned.fill(
          child: ColoredBox(color: OverlayTokens.scrimFloor),
        ),
        SafeArea(
          child: Stack(
            fit: StackFit.expand,
            children: [
              Padding(
                padding: const EdgeInsets.only(top: 56),
                child: ExperienceChatPane(
                  experienceId: widget.experienceId,
                  accentColor: widget.accentColor,
                  paneHeight: MediaQuery.of(context).size.height,
                  onAddMedia: widget.onAddMedia,
                  onDeleteMedia: widget.onDeleteMedia,
                  onReorderMedia: widget.onReorderMedia,
                ),
              ),
              Positioned(
                top: 0,
                left: 0,
                right: 0,
                height: 88,
                child: IgnorePointer(
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      gradient: LinearGradient(
                        begin: Alignment.topCenter,
                        end: Alignment.bottomCenter,
                        colors: [
                          Colors.black.withValues(alpha: 0.65),
                          Colors.black.withValues(alpha: 0.35),
                          Colors.black.withValues(alpha: 0),
                        ],
                        stops: const [0.0, 0.6, 1.0],
                      ),
                    ),
                  ),
                ),
              ),
              Positioned(
                top: 0,
                left: 0,
                right: 0,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 16,
                    vertical: 8,
                  ),
                  child: Row(
                    children: [BackButtonWidget(onPressed: _close)],
                  ),
                ),
              ),
            ],
          ),
        ),
      ],
    );
  }
}
