import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/router/video_background_route_observer.dart';
import 'package:ripls/core/utils/video_cache_helper.dart';
import 'package:ripls/presentation/widgets/media/media_background.dart';
import 'package:ripls/services/providers.dart';
import 'package:video_player/video_player.dart';

final _log = Logger('VideoBackgroundHost');

/// Owns the [VideoPlayerController] for a content view background, ensuring
/// disposal happens after the widget is removed from the tree.
class VideoBackgroundHost extends ConsumerStatefulWidget {
  final String? mediaPath;
  final String? mediaId;
  final bool isVideo;
  final String? thumbnailUrl;
  final bool isMuted;

  const VideoBackgroundHost({
    super.key,
    this.mediaPath,
    this.mediaId,
    this.isVideo = false,
    this.thumbnailUrl,
    this.isMuted = true,
  });

  @override
  ConsumerState<VideoBackgroundHost> createState() =>
      _VideoBackgroundHostState();
}

class _VideoBackgroundHostState extends ConsumerState<VideoBackgroundHost>
    with WidgetsBindingObserver, RouteAware {
  VideoPlayerController? _controller;
  String? _loadedMediaId;
  bool _loading = false;
  final Set<String> _shownErrorMediaIds = <String>{};
  // Tracks whether playback was deliberately paused by a route or
  // lifecycle event so we know whether to resume on return. Without this
  // a video the user manually paused (future enhancement) would be
  // force-resumed every time the screen comes back into view.
  bool _pausedByVisibility = false;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _maybeLoadController();
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final route = ModalRoute.of(context);
    if (route is PageRoute) {
      videoBackgroundRouteObserver.subscribe(this, route);
    }
  }

  @override
  void didPushNext() {
    // Another full-screen route was pushed on top — pause so its audio
    // doesn't fight ours, and so backgrounded audio doesn't keep playing
    // while a different screen is visible.
    _pauseForVisibility();
  }

  @override
  void didPopNext() {
    // The covering route was popped — we're visible again, resume if we
    // were the one to pause.
    _resumeFromVisibility();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    super.didChangeAppLifecycleState(state);
    switch (state) {
      case AppLifecycleState.paused:
      case AppLifecycleState.inactive:
      case AppLifecycleState.hidden:
        _pauseForVisibility();
      case AppLifecycleState.resumed:
        _resumeFromVisibility();
      case AppLifecycleState.detached:
        break;
    }
  }

  void _pauseForVisibility() {
    final controller = _controller;
    if (controller == null || !controller.value.isInitialized) return;
    if (!controller.value.isPlaying) return;
    controller.pause();
    _pausedByVisibility = true;
  }

  void _resumeFromVisibility() {
    final controller = _controller;
    if (!_pausedByVisibility) return;
    _pausedByVisibility = false;
    if (controller == null || !controller.value.isInitialized) return;
    controller.play();
  }

  @override
  void didUpdateWidget(VideoBackgroundHost oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.mediaId != oldWidget.mediaId ||
        widget.isVideo != oldWidget.isVideo) {
      _maybeLoadController();
    }
    if (widget.isMuted != oldWidget.isMuted &&
        _controller != null &&
        _controller!.value.isInitialized) {
      _controller!.setVolume(widget.isMuted ? 0.0 : 1.0);
    }
  }

  Future<void> _maybeLoadController() async {
    if (!widget.isVideo ||
        widget.mediaId == null ||
        widget.mediaPath == null) {
      _disposeController();
      if (mounted) setState(() => _loadedMediaId = null);
      return;
    }
    if (_loadedMediaId == widget.mediaId && _controller != null) return;
    if (_loading) return;

    _loading = true;
    try {
      final repo = ref.read(mediaRepositoryProvider);
      final newController = await createCachedVideoController(
        repo,
        widget.mediaId!,
        widget.mediaPath!,
      );
      if (!mounted) {
        await newController.dispose();
        return;
      }
      await newController.setVolume(widget.isMuted ? 0.0 : 1.0);
      _disposeController();
      _controller = newController;
      _loadedMediaId = widget.mediaId;
      setState(() {});
    } catch (e, stackTrace) {
      _log.severe('Failed to load video controller: $e', e, stackTrace);
      _maybeShowLoadFailureSnackbar();
    } finally {
      _loading = false;
    }
  }

  void _maybeShowLoadFailureSnackbar() {
    if (!mounted) return;
    final mediaId = widget.mediaId;
    if (mediaId == null || !_shownErrorMediaIds.add(mediaId)) return;
    final messenger = ScaffoldMessenger.maybeOf(context);
    if (messenger == null) return;
    messenger.showSnackBar(
      SnackBar(content: Text(context.l10n.mediaVideoLoadFailed)),
    );
  }

  void _disposeController() {
    _controller?.dispose();
    _controller = null;
  }

  @override
  void dispose() {
    videoBackgroundRouteObserver.unsubscribe(this);
    WidgetsBinding.instance.removeObserver(this);
    _disposeController();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return MediaBackground(
      mediaPath: widget.mediaPath,
      mediaId: widget.mediaId,
      isVideo: widget.isVideo,
      videoController: _controller,
      thumbnailUrl: widget.thumbnailUrl,
    );
  }
}
