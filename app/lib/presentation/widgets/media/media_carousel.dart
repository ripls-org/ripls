import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:gal/gal.dart';
import 'package:http/http.dart' as http;
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/models/media_item_data.dart';
import 'package:ripls/core/router/video_background_route_observer.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/core/utils/video_cache_helper.dart';
import 'package:ripls/data/repositories/media_repository.dart';
import 'package:ripls/presentation/viewmodels/video_audio_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_add_media_page.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_header.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_image_viewer.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_media_item.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_thumbnail_strip.dart';
import 'package:ripls/presentation/widgets/media/carousel/carousel_video_player.dart';
import 'package:ripls/presentation/widgets/media/video_mute_toggle_button.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/presentation/widgets/web/web_unsupported.dart';
import 'package:ripls/services/providers.dart';
import 'package:video_player/video_player.dart';

// Re-export MediaItem so callers that import this file continue to work.
export 'package:ripls/presentation/widgets/media/carousel/carousel_media_item.dart'
    show MediaItem;

/// MediaCarousel displays a full-screen carousel of media items (images and videos).
class MediaCarousel extends ConsumerStatefulWidget {
  final List<MediaItem> Function() getMediaItems;
  final int initialIndex;
  final VoidCallback onClose;
  final Future<void> Function()? onAddMedia;
  // onDeleteMedia: handles deletion for owners and uploaders. Pass null if
  // deletion is not supported for this context at all.
  final Future<void> Function(String mediaId)? onDeleteMedia;
  // isOwner: when true, the delete button appears on all items regardless of
  // who uploaded them. When false, delete only appears on items the current
  // user uploaded (checked against item.uploader.id).
  final bool isOwner;
  // onReorderMedia: only the owner can reorder — pass null to disable.
  final Future<void> Function(List<String> mediaIds)? onReorderMedia;

  const MediaCarousel({
    super.key,
    required this.getMediaItems,
    this.initialIndex = 0,
    required this.onClose,
    this.onAddMedia,
    this.onDeleteMedia,
    this.isOwner = false,
    this.onReorderMedia,
  });

  /// show displays the media carousel as a full-screen overlay.
  static void show({
    required BuildContext context,
    required List<MediaItem> Function() getMediaItems,
    int initialIndex = 0,
    Future<void> Function()? onAddMedia,
    Future<void> Function(String mediaId)? onDeleteMedia,
    bool isOwner = false,
    Future<void> Function(List<String> mediaIds)? onReorderMedia,
  }) {
    NavigationHelpers.pushScreen(
      context: context,
      routeName: 'media_viewer',
      screen: MediaCarousel(
        getMediaItems: getMediaItems,
        initialIndex: initialIndex,
        // onClose is intentionally left as a no-op here. The carousel's own
        // performClose() pops using its live context instead of this
        // potentially-stale caller context.
        onClose: () {},
        onAddMedia: onAddMedia,
        onDeleteMedia: onDeleteMedia,
        isOwner: isOwner,
        onReorderMedia: onReorderMedia,
      ),
    );
  }

  @override
  ConsumerState<MediaCarousel> createState() => _MediaCarouselState();
}

class _MediaCarouselState extends ConsumerState<MediaCarousel>
    with
        SingleTickerProviderStateMixin,
        SwipeToCloseMixin,
        WidgetsBindingObserver,
        RouteAware {
  late PageController _pageController;
  late int _currentIndex;
  final Map<int, VideoPlayerController> _videoControllers = {};
  late List<MediaItem> _mediaItems;
  bool _isAddingMedia = false;
  bool _isEditMode = false;
  bool _isDownloading = false;
  bool _isSaving = false;
  // Carousel is a deliberate viewing surface (unlike the muted-by-default
  // feed), so videos here start with audio on. The volume toggle in the
  // header lets users mute mid-session.
  bool _isMuted = false;
  // Tracks whether the currently-visible video was deliberately paused by
  // a route or lifecycle event so we know whether to resume on return.
  bool _pausedByVisibility = false;

  int get _totalPages =>
      _mediaItems.length + (widget.onAddMedia != null ? 1 : 0);
  bool get _canReorder => widget.onReorderMedia != null;

  bool _canDeleteItem(MediaItem item) {
    if (widget.onDeleteMedia == null) return false;
    if (widget.isOwner) return true;
    final currentUserId = ref.read(authStateProvider).user?.id ?? '';
    return currentUserId.isNotEmpty &&
        item.uploader != null &&
        item.uploader!.id == currentUserId;
  }

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _mediaItems = widget.getMediaItems();
    _currentIndex = widget.initialIndex;
    _pageController = PageController(initialPage: widget.initialIndex);
    _initializeCurrentVideo();
    // Inform the audio-session manager that we want audible playback while
    // the carousel is open — without this, video_player's default `.playback`
    // category still silently hijacks other apps' audio. Writing through
    // videoUnmutedProvider keeps the session-wide flag (#1250) consistent
    // with what the carousel is actually doing.
    if (!_isMuted) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (!mounted) return;
        ref.read(videoUnmutedProvider.notifier).set(true);
      });
    }
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
  void didPushNext() => _pauseForVisibility();

  @override
  void didPopNext() => _resumeFromVisibility();

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
    final controller = _videoControllers[_currentIndex];
    if (controller == null || !controller.value.isInitialized) return;
    if (!controller.value.isPlaying) return;
    controller.pause();
    _pausedByVisibility = true;
  }

  void _resumeFromVisibility() {
    if (!_pausedByVisibility) return;
    _pausedByVisibility = false;
    final controller = _videoControllers[_currentIndex];
    if (controller == null || !controller.value.isInitialized) return;
    controller.play();
  }

  void _refreshMediaItems() {
    setState(() {
      _mediaItems = widget.getMediaItems();
    });
  }

  @override
  void dispose() {
    videoBackgroundRouteObserver.unsubscribe(this);
    WidgetsBinding.instance.removeObserver(this);
    _pageController.dispose();
    for (final controller in _videoControllers.values) {
      controller.dispose();
    }
    super.dispose();
  }

  @override
  void performClose() {
    // Pop using the carousel's own live context rather than the captured
    // caller context stored in widget.onClose, which may be stale by the
    // time the async slide-out animation completes.
    if (context.mounted) {
      Navigator.of(context).pop();
    }
  }

  Future<void> _initializeCurrentVideo() async {
    if (_currentIndex >= _mediaItems.length) return;

    final item = _mediaItems[_currentIndex];
    if (item.isVideo && !_videoControllers.containsKey(_currentIndex)) {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final cacheKey = item.mediaId ?? item.url;
      // When the video has a server-side mediaId, resolve a fresh
      // presigned URL at viewer-open time instead of reusing item.url
      // (which may have been captured into a tap closure long enough
      // ago to be past its 35-minute expiry — see #1833). On disk-cache
      // hits inside getVideoFile, the URL is unused; the only network
      // cost on the warm path is one metadata-cache lookup.
      final url = await _resolveVideoUrl(mediaRepository, item);
      final controller = await createCachedVideoController(
        mediaRepository,
        cacheKey,
        url,
      );
      // createCachedVideoController mutes by default for the feed-background
      // use case; the carousel is a deliberate view, so apply the current
      // local mute state before handing the controller to the UI.
      await controller.setVolume(_isMuted ? 0.0 : 1.0);

      if (mounted) {
        setState(() {
          _videoControllers[_currentIndex] = controller;
        });
      }
    }
  }

  void _toggleMute() {
    setState(() {
      _isMuted = !_isMuted;
    });
    final volume = _isMuted ? 0.0 : 1.0;
    for (final controller in _videoControllers.values) {
      if (controller.value.isInitialized) {
        controller.setVolume(volume);
      }
    }
    // Keep the session-wide audio flag in sync so the audio session category
    // (ambient ↔ playback+duckOthers) tracks what the user can hear.
    ref.read(videoUnmutedProvider.notifier).set(!_isMuted);
  }

  bool get _currentItemIsVideo {
    if (_currentIndex >= _mediaItems.length) return false;
    return _mediaItems[_currentIndex].isVideo;
  }

  Future<String> _resolveVideoUrl(
      MediaRepository mediaRepository, MediaItem item) async {
    final mediaId = item.mediaId;
    if (mediaId == null || mediaId.isEmpty) return item.url;
    try {
      final fresh = await mediaRepository.getFullMediaUrl(mediaId);
      return fresh.url.isNotEmpty ? fresh.url : item.url;
    } catch (_) {
      // If the metadata lookup itself fails, fall through to the
      // captured URL — getVideoFile will then hit the disk cache when
      // possible, or surface the network error to the player.
      return item.url;
    }
  }

  void _onPageChanged(int index) {
    setState(() {
      // Pause previous video if it exists.
      final previousController = _videoControllers[_currentIndex];
      previousController?.pause();

      _currentIndex = index;
    });

    // Initialize and play new video if needed.
    _initializeCurrentVideo();
  }

  void _goToPreviousPage() {
    if (_currentIndex > 0) {
      _pageController.previousPage(
        duration: accessibleDuration(context, const Duration(milliseconds: 300)),
        curve: Curves.easeInOut,
      );
    }
  }

  void _goToNextPage() {
    if (_currentIndex < _totalPages - 1) {
      _pageController.nextPage(
        duration: accessibleDuration(context, const Duration(milliseconds: 300)),
        curve: Curves.easeInOut,
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return buildSwipeableContent(
      child: Scaffold(
        backgroundColor: Colors.black,
        body: Stack(
          children: [
            // Main column: header spacer → preview → context bar → thumbnails
            Column(
              children: [
                // Reserve space for the header overlay.
                SafeArea(
                  bottom: false,
                  child: const SizedBox(height: 56),
                ),
                // Preview fills remaining space.
                Expanded(
                  child: Stack(
                    children: [
                      PageView.builder(
                        controller: _pageController,
                        onPageChanged: _onPageChanged,
                        itemCount: _totalPages,
                        itemBuilder: (context, index) {
                          if (widget.onAddMedia != null &&
                              index == _mediaItems.length) {
                            return CarouselAddMediaPage(
                              onUpload: _handleAddMedia,
                            );
                          }
                          final item = _mediaItems[index];
                          return _buildMediaItem(item, index);
                        },
                      ),

                      // (Volume toggle lives in the bottom context bar,
                      // immediately left of the save/download button.)

                      // Previous arrow (left side).
                      if (_currentIndex > 0)
                        Positioned(
                          left: 4,
                          top: 0,
                          bottom: 0,
                          child: Center(
                            child: Tappable(
                              semanticsLabel:
                                  context.l10n.a11yMediaPrevious,
                              onTap: _goToPreviousPage,
                              child: Container(
                                padding: const EdgeInsets.all(8),
                                decoration: BoxDecoration(
                                  color:
                                      Colors.black.withValues(alpha: 0.25),
                                  shape: BoxShape.circle,
                                ),
                                child: Icon(
                                  Icons.chevron_left,
                                  color:
                                      Colors.white.withValues(alpha: 0.7),
                                  size: 28,
                                ),
                              ),
                            ),
                          ),
                        ),

                      // Next arrow (right side).
                      if (_currentIndex < _totalPages - 1)
                        Positioned(
                          right: 4,
                          top: 0,
                          bottom: 0,
                          child: Center(
                            child: Tappable(
                              semanticsLabel: context.l10n.a11yMediaNext,
                              onTap: _goToNextPage,
                              child: Container(
                                padding: const EdgeInsets.all(8),
                                decoration: BoxDecoration(
                                  color:
                                      Colors.black.withValues(alpha: 0.25),
                                  shape: BoxShape.circle,
                                ),
                                child: Icon(
                                  Icons.chevron_right,
                                  color:
                                      Colors.white.withValues(alpha: 0.7),
                                  size: 28,
                                ),
                              ),
                            ),
                          ),
                        ),
                    ],
                  ),
                ),

                // Context bar: attribution + save + delete.
                if (_mediaItems.isNotEmpty) _buildContextBar(),

                // Thumbnail carousel.
                if (_mediaItems.isNotEmpty)
                  SafeArea(
                    top: false,
                    child: CarouselThumbnailStrip(
                      items: _mediaItemsAsData(),
                      currentIndex: _currentIndex,
                      isEditMode: _isEditMode,
                      isAddingMedia: _isAddingMedia,
                      canReorder: _canReorder,
                      showAddButton: widget.onAddMedia != null,
                      onThumbnailTap: (index) {
                        _pageController.animateToPage(
                          index,
                          duration: accessibleDuration(context, const Duration(milliseconds: 300)),
                          curve: Curves.easeInOut,
                        );
                      },
                      onReorderItem: _handleReorder,
                      onDeleteThumbnail: _handleDeleteMedia,
                      onAddTap: _handleAddMedia,
                      onLongPress: () =>
                          setState(() => _isEditMode = true),
                    ),
                  ),
              ],
            ),

            // Header overlay on top.
            SafeArea(
              child: CarouselHeader(
                itemCount: _mediaItems.length,
                isEditMode: _isEditMode,
                isDownloading: _isDownloading,
                hasItems: _mediaItems.isNotEmpty,
                onClose: handleClose,
                onToggleEditMode: () =>
                    setState(() => _isEditMode = false),
                onDownloadAll: _handleDownloadAll,
              ),
            ),
          ],
        ),
      ),
    );
  }

  /// _buildContextBar renders the attribution and action bar for the current item.
  Widget _buildContextBar() {
    if (_currentIndex >= _mediaItems.length) return const SizedBox.shrink();
    final item = _mediaItems[_currentIndex];
    return CarouselContextBar(
      item: item,
      isSaving: _isSaving,
      canDelete: _canDeleteItem(item),
      onSave: () => _saveCurrentMedia(item.mediaId!),
      onDelete: () => _handleDeleteMedia(item.mediaId!),
      muteButton: _currentItemIsVideo
          ? VideoMuteToggleButton(
              isMuted: _isMuted,
              onTap: _toggleMute,
            )
          : null,
    );
  }

  /// _buildMediaItem delegates to the appropriate media-kind widget.
  Widget _buildMediaItem(MediaItem item, int index) {
    if (item.isVideo) {
      return CarouselVideoPlayer(controller: _videoControllers[index]);
    }
    return CarouselImageViewer(
      item: item,
      onTap: () => _showFullscreenImage(item),
    );
  }

  /// _showFullscreenImage pushes a fullscreen image overlay.
  void _showFullscreenImage(MediaItem item) {
    Navigator.of(context).push(
      PageRouteBuilder(
        opaque: false,
        barrierColor: Colors.black,
        pageBuilder: (context, animation, secondaryAnimation) {
          return FullscreenImageViewer(
            mediaId: item.mediaId,
            imageUrl: item.url,
            attribution: item.attribution,
          );
        },
        transitionsBuilder: (context, animation, secondaryAnimation, child) {
          return FadeTransition(opacity: animation, child: child);
        },
      ),
    );
  }

  /// _mediaItemsAsData converts the current [_mediaItems] list to
  /// [MediaItemData] for use by [CarouselThumbnailStrip].
  List<MediaItemData> _mediaItemsAsData() {
    return _mediaItems.map((item) {
      return MediaItemData(
        id: item.mediaId ?? '',
        url: item.url,
        contentType: item.contentType ?? 'image/jpeg',
        isVideo: item.isVideo,
        uploader: item.uploader,
        attribution: item.attribution,
        uploadedAtUnixSec: item.uploadedAtUnixSec,
      );
    }).toList();
  }

  Future<void> _handleDownloadAll() async {
    // gal (gallery save) and dart:io File operations have no web
    // implementation; the bulk-download flow can't write to a local
    // gallery on web. See #2157.
    if (WebUnsupported.showPhotoUploadNotice(context)) return;
    setState(() => _isDownloading = true);
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final items = _mediaItems.where((item) => item.mediaId != null).toList();
      for (final item in items) {
        await _downloadMediaItem(
            mediaRepository, item.mediaId!, item.url, item.isVideo);
      }
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.mediaDownloadComplete)),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(context.l10n.mediaDownloadFailed),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _isDownloading = false);
    }
  }

  Future<void> _saveCurrentMedia(String mediaId) async {
    // gal (gallery save) and dart:io File operations have no web
    // implementation. The browser itself handles media save via
    // right-click / long-press → save. Show a notice pointing the
    // visitor at the mobile app for the in-app one-tap save flow.
    if (WebUnsupported.showPhotoUploadNotice(context)) return;
    setState(() => _isSaving = true);
    try {
      final mediaRepository = ref.read(mediaRepositoryProvider);
      final index = _mediaItems.indexWhere((item) => item.mediaId == mediaId);
      final isVideo = index >= 0 ? _mediaItems[index].isVideo : false;
      final url = index >= 0 ? _mediaItems[index].url : '';
      await _downloadMediaItem(mediaRepository, mediaId, url, isVideo);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.mediaSaved)),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(context.l10n.mediaDownloadFailed),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _isSaving = false);
    }
  }

  Future<void> _downloadMediaItem(
    MediaRepository mediaRepository,
    String mediaId,
    String url,
    bool isVideo,
  ) async {
    if (isVideo) {
      final file = await mediaRepository.getVideoFile(mediaId, url);
      await Gal.putVideo(file.path);
    } else {
      final media = await mediaRepository.get(mediaId);
      final response = await http.get(Uri.parse(media.url));
      if (response.statusCode == 200) {
        final tempDir = await Directory.systemTemp.createTemp('media_download');
        final ext = media.contentType.contains('png') ? 'png' : 'jpg';
        final tempFile = File('${tempDir.path}/media_$mediaId.$ext');
        await tempFile.writeAsBytes(response.bodyBytes);
        await Gal.putImage(tempFile.path);
        await tempDir.delete(recursive: true);
      }
    }
  }

  Future<void> _handleAddMedia() async {
    if (widget.onAddMedia == null || _isAddingMedia) return;

    setState(() => _isAddingMedia = true);

    try {
      await widget.onAddMedia!();

      if (!mounted) return;

      // Riverpod schedules the widget rebuild for the next frame when state
      // changes. addPostFrameCallback fires at the END of the current frame
      // (the same frame the rebuild is scheduled), so we need to wait for two
      // frames: one for Riverpod to rebuild InlineConversationView with the
      // updated mediaItems, and one more to ensure the new widget.mediaItems
      // prop is readable through the getMediaItems closure.
      for (var i = 0; i < 2; i++) {
        final completer = Completer<void>();
        WidgetsBinding.instance
            .addPostFrameCallback((_) => completer.complete());
        await completer.future;
        if (!mounted) return;
      }

      // Refresh media items — this calls setState so the PageView itemCount
      // updates, but the PageView hasn't rebuilt yet. Wait one more frame so
      // the new page slot exists before we animate to it.
      _refreshMediaItems();

      final refreshCompleter = Completer<void>();
      WidgetsBinding.instance
          .addPostFrameCallback((_) => refreshCompleter.complete());
      await refreshCompleter.future;
      if (!mounted) return;

      // Navigate to the newly added media (last item).
      if (_mediaItems.isNotEmpty) {
        await _pageController.animateToPage(
          _mediaItems.length - 1,
          duration: accessibleDuration(context, const Duration(milliseconds: 300)),
          curve: Curves.easeInOut,
        );
      }
    } finally {
      if (mounted) setState(() => _isAddingMedia = false);
    }
  }

  Future<void> _handleDeleteMedia(String mediaId) async {
    if (widget.onDeleteMedia == null) return;

    // Show confirmation dialog.
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(context.l10n.mediaDeleteTitle),
        content: Text(context.l10n.mediaDeleteConfirmation),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(context.l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            style: TextButton.styleFrom(foregroundColor: Colors.red),
            child: Text(context.l10n.commonDelete),
          ),
        ],
      ),
    );

    if (confirmed != true) return;

    try {
      await widget.onDeleteMedia!(mediaId);

      if (!mounted) return;

      // Remove the item directly from local state. The carousel is an opaque
      // pushed route, so the parent route's widget tree may not rebuild while
      // it is obscured — relying on widget.getMediaItems() would see stale
      // data.
      setState(() {
        _mediaItems.removeWhere((item) => item.mediaId == mediaId);
        // Clamp index so it stays in bounds after removal.
        if (_currentIndex >= _mediaItems.length && _currentIndex > 0) {
          _currentIndex = _mediaItems.length - 1;
        }
      });

      if (_mediaItems.isEmpty) {
        if (mounted) Navigator.of(context).pop();
        return;
      }

      // The PageView is driven by the PageController's physical scroll
      // position, not by _currentIndex. After item removal the controller
      // offset may point at a now-missing page slot (e.g. we deleted the last
      // item) or at the same slot whose content has changed. jumpToPage forces
      // the PageView to re-render at the correct position in either case.
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) {
          _pageController.jumpToPage(_currentIndex);
        }
      });
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Failed to delete media: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    }
  }

  void _handleReorder(int oldIndex, int newIndex) async {
    // Indices come from ReorderableListView.onReorderItem, which already
    // adjusts newIndex for the removed item, so no manual decrement is needed.

    // Don't allow reordering if the target is the add button.
    if (newIndex >= _mediaItems.length || oldIndex >= _mediaItems.length) {
      return;
    }

    setState(() {
      // Reorder locally for immediate feedback.
      final item = _mediaItems.removeAt(oldIndex);
      _mediaItems.insert(newIndex, item);

      // Update current index if needed.
      if (_currentIndex == oldIndex) {
        _currentIndex = newIndex;
      } else if (oldIndex < _currentIndex && newIndex >= _currentIndex) {
        _currentIndex--;
      } else if (oldIndex > _currentIndex && newIndex <= _currentIndex) {
        _currentIndex++;
      }
    });

    // Call the callback with the new order.
    if (widget.onReorderMedia != null) {
      final mediaIds = _mediaItems
          .map((item) => item.mediaId ?? '')
          .where((id) => id.isNotEmpty)
          .toList();

      try {
        await widget.onReorderMedia!(mediaIds);
      } catch (e) {
        // Revert on error.
        if (mounted) {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Failed to reorder media: $e'),
              backgroundColor: Colors.red,
            ),
          );
          // Refresh to get the original order back.
          _refreshMediaItems();
        }
      }
    }
  }
}
