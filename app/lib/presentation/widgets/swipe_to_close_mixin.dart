import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_wrapper.dart';

/// Mixin that provides swipe-to-close functionality with slide animation.
///
/// This mixin handles the common pattern of:
/// - Slide animation from right (Offset(1.0, 0.0) to Offset.zero)
/// - SwipeToCloseWrapper integration
/// - Reverse animation on close
///
/// **Pattern A -- Standard screens with AppBar:**
/// ```dart
/// class _MyScreenState extends ConsumerState<MyScreen>
///     with SingleTickerProviderStateMixin, SwipeToCloseMixin {
///   @override
///   Widget build(BuildContext context) {
///     return buildSwipeableScaffold(
///       appBar: AppBar(
///         leading: AppBarBackButton(onPressed: handleClose),
///         title: Text('My Screen'),
///       ),
///       body: MyContent(),
///     );
///   }
/// }
/// ```
///
/// **Pattern B -- Overlay screens without AppBar:**
/// ```dart
/// class _GearScreenState extends ConsumerState<GearScreen>
///     with SingleTickerProviderStateMixin, SwipeToCloseMixin {
///   @override
///   Widget build(BuildContext context) {
///     return buildSwipeableContent(
///       child: Scaffold(
///         backgroundColor: Colors.black,
///         body: SafeArea(
///           child: Stack(children: [
///             ContentView(...),
///             BackButtonWidget(onPressed: handleClose),
///           ]),
///         ),
///       ),
///     );
///   }
/// }
/// ```
///
/// **Custom close behavior:**
/// Override [performClose] to run custom logic (e.g., callbacks) instead of
/// the default `Navigator.of(context).pop()`:
/// ```dart
/// @override
/// void performClose() {
///   widget.onReturn();
///   Navigator.of(context).pop();
/// }
/// ```
mixin SwipeToCloseMixin<T extends StatefulWidget> on State<T>, TickerProvider {
  late AnimationController _slideAnimationController;
  late Animation<Offset> _slideAnimation;

  /// Initializes the slide animation.
  /// Call this from your initState() using super.initState().
  ///
  /// The duration is resolved in [didChangeDependencies] (after the widget
  /// has access to the MediaQuery for accessibleDuration) so the controller
  /// honours the system reduce-motion setting. We start at zero here, then
  /// update + forward once dependencies settle.
  @override
  void initState() {
    super.initState();
    _slideAnimationController = AnimationController(
      duration: Duration.zero,
      vsync: this,
    );
    _slideAnimation = Tween<Offset>(
      begin: const Offset(1, 0), // Start from right
      end: Offset.zero, // End at normal position
    ).animate(
      CurvedAnimation(
        parent: _slideAnimationController,
        curve: Curves.easeInOut,
      ),
    );
  }

  bool _slideStarted = false;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _slideAnimationController.duration =
        accessibleDuration(context, const Duration(milliseconds: 300));
    if (!_slideStarted) {
      _slideStarted = true;
      _slideAnimationController.forward();
    }
  }

  /// Disposes the animation controller.
  /// Call this from your dispose() using super.dispose().
  @override
  void dispose() {
    _slideAnimationController.dispose();
    super.dispose();
  }

  /// Called after the reverse animation completes to actually close the screen.
  ///
  /// Override this to perform custom close logic (e.g., calling a callback
  /// before popping). The default implementation tries `Navigator.pop()` if
  /// possible, otherwise falls back to GoRouter navigation to home.
  ///
  /// The fallback handles deep link scenarios where the screen was navigated
  /// to directly without a navigation stack (e.g., via invite link).
  void performClose() {
    final navigator = Navigator.of(context);
    if (navigator.canPop()) {
      navigator.pop();
    } else {
      // No navigation stack (e.g., deep link) - use GoRouter to go home
      context.go('/');
    }
  }

  /// Handles closing the screen with reverse animation.
  /// Pass this to AppBarBackButton, BackButtonWidget, and SwipeToCloseWrapper.
  Future<void> handleClose() async {
    // Animate out before closing
    await _slideAnimationController.reverse();
    if (mounted) {
      performClose();
    }
  }

  /// Builds arbitrary content wrapped with swipe-to-close and slide animation.
  ///
  /// Use this for overlay screens (Pattern B) that don't use a standard
  /// AppBar -- for example, screens with a floating [BackButtonWidget] over
  /// media content. The [child] is typically a [Scaffold] with a [Stack] body.
  Widget buildSwipeableContent({required Widget child}) {
    return SwipeToCloseWrapper(
      onClose: handleClose,
      child: SlideTransition(
        position: _slideAnimation,
        child: child,
      ),
    );
  }

  /// Builds a scaffold wrapped with swipe-to-close functionality.
  ///
  /// This is a convenience method that wraps the scaffold in:
  /// SwipeToCloseWrapper > SlideTransition > Scaffold
  ///
  /// Note: The AppBar should include `leading: AppBarBackButton(onPressed: handleClose)`
  /// to maintain consistency with the swipe gesture.
  Widget buildSwipeableScaffold({
    PreferredSizeWidget? appBar,
    required Widget body,
    Color? backgroundColor,
    Widget? floatingActionButton,
    FloatingActionButtonLocation? floatingActionButtonLocation,
    Widget? bottomNavigationBar,
    Widget? drawer,
    Widget? endDrawer,
    bool extendBody = false,
    bool extendBodyBehindAppBar = false,
  }) {
    return SwipeToCloseWrapper(
      onClose: handleClose,
      child: SlideTransition(
        position: _slideAnimation,
        child: Scaffold(
          appBar: appBar,
          body: body,
          backgroundColor: backgroundColor,
          floatingActionButton: floatingActionButton,
          floatingActionButtonLocation: floatingActionButtonLocation,
          bottomNavigationBar: bottomNavigationBar,
          drawer: drawer,
          endDrawer: endDrawer,
          extendBody: extendBody,
          extendBodyBehindAppBar: extendBodyBehindAppBar,
        ),
      ),
    );
  }
}
