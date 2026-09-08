import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:keyboard_actions/keyboard_actions.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show GeocodedLocation;
import 'package:ripls/data/gen/ripls/api/time.pb.dart' show ExperienceTime;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/data/repositories/provisional_user_repository.dart'
    show ProvisionalUser;
import 'package:ripls/presentation/screens/experience/widgets/preview/preview_community_selector.dart';
import 'package:ripls/presentation/screens/experience/widgets/preview/preview_header.dart';
import 'package:ripls/presentation/screens/experience/widgets/preview/preview_location_selector.dart';
import 'package:ripls/presentation/screens/experience/widgets/preview/preview_time_picker_sheet.dart';
import 'package:ripls/presentation/screens/experience/widgets/preview/preview_time_selector.dart';
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/content/content_action_button.dart';
import 'package:ripls/presentation/widgets/content/content_editable_field.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';
import 'package:ripls/presentation/widgets/content/content_error_banner.dart';
import 'package:ripls/presentation/widgets/creation/streaming_skeleton.dart';
import 'package:ripls/presentation/widgets/keyboard_actions_config.dart';
import 'package:ripls/presentation/widgets/keyboard_dismiss_wrapper.dart';
import 'package:ripls/presentation/widgets/media/background_media_image.dart';
import 'package:ripls/presentation/widgets/media/media_picker_button.dart';
import 'package:ripls/presentation/widgets/media/media_picker_dialog.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/services/post_creation_service.dart';
import 'package:ripls/services/providers.dart';
import 'package:video_player/video_player.dart';

export 'package:ripls/presentation/screens/experience/widgets/preview/participant_picker_sheet.dart';
export 'package:ripls/presentation/screens/experience/widgets/preview/preview_time_picker_sheet.dart';

final _log = Logger('ExperiencePreviewModal');

/// ExperiencePreviewModal shows a preview of AI-generated experience suggestions
/// in a full-screen modal overlay. Users can edit the suggestions before creating
/// and sharing the experience.
class ExperiencePreviewModal extends ConsumerStatefulWidget {
  const ExperiencePreviewModal({super.key});

  @override
  ConsumerState<ExperiencePreviewModal> createState() =>
      _ExperiencePreviewModalState();
}

class _ExperiencePreviewModalState extends ConsumerState<ExperiencePreviewModal>
    with ContentEditingMixin {
  String? _locationDisplayText;
  List<String> _selectedCommunityIds = [];
  final _titleFocusNode = FocusNode();
  final _descriptionFocusNode = FocusNode();

  /// Returns true if [time] is a SpecificTime that has already passed.
  bool _isTimeInPast(ExperienceTime? time) {
    if (time == null || !time.hasSpecific()) return false;
    final nowSec = DateTime.now().millisecondsSinceEpoch ~/ 1000;
    return time.specific.unixTimestampSec.toInt() < nowSec;
  }

  @override
  void dispose() {
    _titleFocusNode.dispose();
    _descriptionFocusNode.dispose();
    super.dispose();
  }

  @override
  void initState() {
    super.initState();

    // Initialize from ViewModel state
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final state = ref.read(genExperienceProvider);

      _log.info('ExperiencePreviewModal initState - reading state:');
      _log.info('  currentStep: ${state.currentStep}');
      _log.info('  aiGeneratedName: "${state.aiGeneratedName}"');
      _log.info('  aiGeneratedDescription: "${state.aiGeneratedDescription}"');
      _log.info('  editedName: "${state.editedName}"');
      _log.info('  editedDescription: "${state.editedDescription}"');
      _log.info('  selectedMediaIds: ${state.selectedMediaIds}');
      _log.info('  locationQuery: ${state.locationQuery}');
      _log.info('  geocodedLocation: ${state.geocodedLocation?.name}');

      // Initialize editing buffer with AI-generated or edited values
      final initialName = state.editedName ?? state.aiGeneratedName ?? '';
      final initialDescription =
          state.editedDescription ?? state.aiGeneratedDescription ?? '';

      _log.info('  initialName: "$initialName"');
      _log.info('  initialDescription: "$initialDescription"');

      initializeEditing(
        initialTitle: initialName,
        initialDescription: initialDescription,
      );

      // Update ViewModel with initial values
      if (initialName.isNotEmpty) {
        ref.read(genExperienceProvider.notifier).updateName(initialName);
      }

      if (initialDescription.isNotEmpty) {
        ref
            .read(genExperienceProvider.notifier)
            .updateDescription(initialDescription);
      }

      // Load location display text if available
      _loadLocationDisplayText();

      // Initialize time from AI-extracted values if available
      unawaited(
        ref.read(genExperienceProvider.notifier).initializeTimeFromExtracted(),
      );

      // Initialize community selection from enabled communities
      _selectedCommunityIds = ref.read(communitiesProvider).communityIds;
    });
  }

  void _showMediaPickerDialog() {
    final state = ref.read(genExperienceProvider);
    final hasMedia =
        state.selectedMediaIds != null && state.selectedMediaIds!.isNotEmpty;

    MediaPickerDialog.show(
      context: context,
      hasMedia: hasMedia,
      onVideoTap: () => _handleMediaPick(
        ref.read(genExperienceProvider.notifier).pickAndUploadVideoFromGallery,
      ),
      onPhotoTap: () => _handleMediaPick(
        ref
            .read(genExperienceProvider.notifier)
            .pickAndUploadMultipleImagesFromGallery,
      ),
      onCameraTap: () => _handleMediaPick(
        ref.read(genExperienceProvider.notifier).pickAndUploadImageFromCamera,
      ),
    );
  }

  Future<void> _handleMediaPick(Future<void> Function() pickFunction) async {
    try {
      await pickFunction();
      if (!mounted) return;
      final failedCount = ref
          .read(genExperienceProvider)
          .batchUploadFailedCount;
      if (failedCount != null && failedCount > 0) {
        ToastHelper.showError(
          context,
          context.l10n.mediaBatchUploadPartialFailure(failedCount),
        );
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to upload media: $e');
      }
    }
  }

  Future<void> _handleCreateExperience() async {
    // Validate required fields
    if (editingTitle.trim().isEmpty) {
      ToastHelper.showError(context, 'Experience name is required');
      return;
    }

    if (editingTitle.trim().length < 3) {
      ToastHelper.showError(
        context,
        'Experience name must be at least 3 characters',
      );
      return;
    }

    if (editingDescription.trim().isEmpty) {
      ToastHelper.showError(context, 'Experience description is required');
      return;
    }

    if (editingDescription.trim().length < 10) {
      ToastHelper.showError(
        context,
        'Experience description must be at least 10 characters',
      );
      return;
    }

    if (_selectedCommunityIds.isEmpty) {
      ToastHelper.showError(context, context.l10n.communitySelectRequired);
      return;
    }

    try {
      await ref
          .read(genExperienceProvider.notifier)
          .createAndShareExperience(_selectedCommunityIds);

      if (!mounted) return;

      final finalState = ref.read(genExperienceProvider);
      if (finalState.hasError) {
        ToastHelper.showError(
          context,
          finalState.error == null
              ? 'Failed to create experience'
              : RpcErrorHandler.localize(finalState.error!, context.l10n),
        );
      } else if (finalState.isCompleted) {
        final experienceId = finalState.createdExperienceId;
        final isPastEvent = _isTimeInPast(finalState.selectedTime);
        final notifier = ref.read(genExperienceProvider.notifier);

        // For past events: resolve AI-mentioned names into participant IDs and
        // provisional users before closing, so the feed can open the completion modal
        // on top with pre-populated attendees.
        List<User> preTaggedUsers = [];
        List<ProvisionalUser> preProvisionalUsers = [];
        if (isPastEvent && experienceId != null) {
          final resolved = await notifier.resolveParticipantsForCompletion();
          if (!mounted) return;
          preTaggedUsers = resolved?.users ?? [];
          preProvisionalUsers = resolved?.provisionalUsers ?? [];
        }

        // Reset provider state while we have stable context
        notifier.reset();

        // Handle post-creation (feed refresh + navigation) while mounted
        await ref.read(postCreationServiceProvider).handlePostCreation();

        if (!mounted) return;

        // Pop — for past events, return ExperienceCreationResult so the feed
        // layer opens the completion modal on top. For future events, pass
        // just the experience ID string for backwards compatibility.
        if (isPastEvent && experienceId != null) {
          Navigator.of(context).pop(
            ExperienceCreationResult(
              experienceId: experienceId,
              isPastEvent: true,
              preTaggedUsers: preTaggedUsers,
              preProvisionalUsers: preProvisionalUsers,
            ),
          );
        } else if (mounted) {
          Navigator.of(context).pop(experienceId);
        }
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to create experience: $e');
      }
    }
  }

  void _handleCancel() {
    Navigator.of(context).pop(); // Returns null (cancelled)
  }

  String _getTimeDisplayText(GenExperienceState state) {
    if (state.selectedTime == null) {
      return 'TBD';
    }

    final time = state.selectedTime!;
    if (time.hasTbd()) {
      return 'TBD';
    } else if (time.hasSpecific()) {
      final timestamp = time.specific.unixTimestampSec.toInt();
      final date = DateTime.fromMillisecondsSinceEpoch(timestamp * 1000);

      // Format as "DayOfWeek, Month Day at H:MM AM/PM"
      final weekdays = [
        'Monday',
        'Tuesday',
        'Wednesday',
        'Thursday',
        'Friday',
        'Saturday',
        'Sunday',
      ];
      final months = [
        'January',
        'February',
        'March',
        'April',
        'May',
        'June',
        'July',
        'August',
        'September',
        'October',
        'November',
        'December',
      ];

      final dayOfWeek = weekdays[date.weekday - 1];
      final month = months[date.month - 1];
      final day = date.day;

      // Convert to 12-hour format
      final hour = date.hour == 0
          ? 12
          : (date.hour > 12 ? date.hour - 12 : date.hour);
      final minute = date.minute.toString().padLeft(2, '0');
      final period = date.hour >= 12 ? 'PM' : 'AM';

      return '$dayOfWeek, $month $day at $hour:$minute $period';
    } else if (time.hasRange()) {
      return time.range.description.isNotEmpty
          ? time.range.description
          : 'Custom range';
    }

    return 'TBD';
  }

  Future<void> _showTimePicker() async {
    final state = ref.read(genExperienceProvider);

    final result = await showAccessibleModal<ExperienceTime>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => PreviewTimePickerSheet(initialTime: state.selectedTime),
    );

    if (result != null && mounted) {
      ref.read(genExperienceProvider.notifier).updateTime(result);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(genExperienceProvider);
    final effectiveMediaIds = state.selectedMediaIds ?? [];
    final firstMediaId = effectiveMediaIds.isNotEmpty
        ? effectiveMediaIds.first
        : null;

    // Streaming generation lands title / description into genExperienceProvider
    // after initState has already snapshotted the (empty) buffer. Sync the
    // editing buffer with the incoming values as long as the user hasn't typed
    // anything of their own in the meantime.
    ref.listen<GenExperienceState>(genExperienceProvider, (prev, next) {
      final incomingName = next.aiGeneratedName;
      if (incomingName != null &&
          incomingName.isNotEmpty &&
          incomingName != editingTitle &&
          editingTitle.isEmpty) {
        updateEditingTitle(incomingName);
        ref.read(genExperienceProvider.notifier).updateName(incomingName);
      }
      final incomingDescription = next.aiGeneratedDescription;
      if (incomingDescription != null &&
          incomingDescription.isNotEmpty &&
          incomingDescription != editingDescription &&
          editingDescription.isEmpty) {
        updateEditingDescription(incomingDescription);
        ref
            .read(genExperienceProvider.notifier)
            .updateDescription(incomingDescription);
      }
    });

    return Dialog.fullscreen(
      child: Scaffold(
        backgroundColor: AppColors.background(context),
        body: KeyboardActions(
          disableScroll: true,
          config: buildKeyboardActionsConfig([
            _titleFocusNode,
            _descriptionFocusNode,
          ]),
          child: KeyboardDismissWrapper(
            child: Stack(
              children: [
                // Background media (video or image) if available
                if (firstMediaId != null) _buildBackground(state, firstMediaId),

                // Gradient overlay
                Positioned.fill(
                  child: Container(
                    decoration: BoxDecoration(
                      gradient: LinearGradient(
                        begin: Alignment.topCenter,
                        end: Alignment.bottomCenter,
                        colors: [
                          Colors.black.withValues(alpha: 0.3),
                          Colors.black.withValues(alpha: 0.7),
                        ],
                      ),
                    ),
                  ),
                ),

                // Content — CustomScrollView + SliverFillRemaining fills
                // the viewport on large screens (Spacer works) but scrolls
                // when content overflows on small screens (iPhone SE).
                SafeArea(
                  child: Padding(
                    padding: const EdgeInsets.all(24),
                    child: CustomScrollView(
                      slivers: [
                        SliverFillRemaining(
                          hasScrollBody: false,
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              // Header
                              const PreviewHeader(),
                              const Spacer(),

                              // Media picker button
                              Padding(
                                padding: const EdgeInsets.only(bottom: 24),
                                child: MediaPickerButton(
                                  hasMedia: firstMediaId != null,
                                  isUploading: state.isUploadingMedia,
                                  onTap: _showMediaPickerDialog,
                                ),
                              ),

                              // Editable experience details
                              _buildExperienceDetails(context),
                              const SizedBox(height: 24),

                              // Error message
                              if (state.hasError) ...[
                                ContentErrorBanner(
                                  errorMessage: RpcErrorHandler.localize(
                                    state.error!,
                                    context.l10n,
                                  ),
                                ),
                                const SizedBox(height: 16),
                              ],

                              // Action button
                              _buildActionButton(context, state),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                ),

                // Close button (top-right)
                if (!state.isLoading)
                  Positioned(
                    right: 12,
                    top: 12,
                    child: SafeArea(
                      child: IconButton(
                        tooltip: context.l10n.a11yClose,
                        onPressed: _handleCancel,
                        icon: const Icon(Icons.close,
                            color: OverlayTokens.textPrimary),
                        style: IconButton.styleFrom(
                          backgroundColor: Colors.black.withValues(alpha: 0.5),
                        ),
                      ),
                    ),
                  ),

                // Mute toggle (top-left) — only visible when video is playing
                if (state.previewVideoController?.value.isInitialized ?? false)
                  Positioned(
                    left: 12,
                    top: 12,
                    child: SafeArea(
                      child: IconButton(
                        tooltip: state.previewIsMuted
                            ? context.l10n.a11yExpUnmute
                            : context.l10n.a11yExpMute,
                        onPressed: () => ref
                            .read(genExperienceProvider.notifier)
                            .togglePreviewMute(),
                        icon: Icon(
                          state.previewIsMuted
                              ? Icons.volume_off
                              : Icons.volume_up,
                          color: OverlayTokens.textPrimary,
                        ),
                        style: IconButton.styleFrom(
                          backgroundColor: Colors.black.withValues(alpha: 0.5),
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  /// Renders the full-screen background — video if the controller is ready,
  /// image otherwise. The image acts as a placeholder while the video loads.
  Widget _buildBackground(GenExperienceState state, String mediaId) {
    final controller = state.previewVideoController;
    if (controller != null && controller.value.isInitialized) {
      return Positioned.fill(
        child: Container(
          color: Colors.black,
          child: SizedBox.expand(
            child: FittedBox(
              fit: BoxFit.cover,
              child: SizedBox(
                width: controller.value.size.width,
                height: controller.value.size.height,
                child: VideoPlayer(controller),
              ),
            ),
          ),
        ),
      );
    }
    return _BackgroundMediaImage(key: ValueKey(mediaId), mediaId: mediaId);
  }

  Widget _buildExperienceDetails(BuildContext context) {
    final state = ref.watch(genExperienceProvider);
    final isStreaming = state.currentStep == GenExperienceStep.generating;
    final titleReady = (state.aiGeneratedName ?? '').isNotEmpty;
    final descriptionReady = (state.aiGeneratedDescription ?? '').isNotEmpty;
    // Time is ready once the AI has emitted any time signal — either an
    // extracted timestamp or a confidence verdict (UNKNOWN counts as
    // resolved). Location is ready when the AI has produced a geocoded
    // result, a location query for the user to confirm, or a selected
    // user-primary-location fallback.
    final timeReady =
        state.extractedTimeUnixSec != null ||
        state.timeConfidence != null ||
        state.selectedTime != null;
    final locationReady =
        state.geocodedLocation != null ||
        state.selectedLocationId != null ||
        (state.locationQuery != null && state.locationQuery!.isNotEmpty);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (isStreaming) ...[
          StreamingGeneratingBanner(label: context.l10n.genStreamingGenerating),
          const SizedBox(height: 12),
        ],
        // Name field using ContentEditableField
        StreamingFieldSlot(
          ready: !isStreaming || titleReady,
          height: 56,
          child: ContentEditableField(
            value: editingTitle,
            label: 'Event Name',
            hintText: 'Event name',
            textFieldKey: const Key('experience_name_field'),
            focusNode: _titleFocusNode,
            onChanged: (value) {
              updateEditingTitle(value);
              ref.read(genExperienceProvider.notifier).updateName(value);
            },
            enabled: !state.isLoading,
            maxLines: 1,
            useOverlayStyle: true,
          ),
        ),
        const SizedBox(height: 12),
        // Description field using ContentEditableField
        StreamingFieldSlot(
          ready: !isStreaming || descriptionReady,
          height: 140,
          child: ContentEditableField(
            value: editingDescription,
            label: 'Description',
            hintText: 'Experience description',
            textFieldKey: const Key('experience_description_field'),
            focusNode: _descriptionFocusNode,
            onChanged: (value) {
              updateEditingDescription(value);
              ref.read(genExperienceProvider.notifier).updateDescription(value);
            },
            enabled: !state.isLoading,
            maxLines: 8,
            minLines: 4,
            useOverlayStyle: true,
          ),
        ),
        const SizedBox(height: 16),
        // Time selector
        StreamingFieldSlot(
          ready: !isStreaming || timeReady,
          height: 56,
          child: PreviewTimeSelector(
            displayText: _getTimeDisplayText(state),
            isPastDate: ref
                .read(genExperienceProvider.notifier)
                .isExtractedDateInPast(),
            isLoading: state.isLoading,
            onTap: _showTimePicker,
          ),
        ),
        const SizedBox(height: 12),
        // Location selector
        StreamingFieldSlot(
          ready: !isStreaming || locationReady,
          height: 56,
          child: PreviewLocationSelector(
            displayText: _getLocationDisplayText(state),
            isLoading: state.isLoading,
            onTap: _showLocationPicker,
            showExtractedQuery:
                state.locationQuery != null &&
                state.locationQuery!.isNotEmpty &&
                state.geocodedLocation == null &&
                state.selectedLocationId == null,
            extractedLocationQuery: state.locationQuery,
          ),
        ),
        const SizedBox(height: 12),
        // Community selector
        PreviewCommunitySelector(
          selectedCommunityIds: _selectedCommunityIds,
          onTap: _openCommunityPicker,
        ),
      ],
    );
  }

  Future<void> _openCommunityPicker() async {
    final result = await CommunitySelectionSheet.showForDeferred(
      context,
      source: CommunitySelectionSource.experienceCreate,
      initialSelection: _selectedCommunityIds,
    );
    if (result != null) setState(() => _selectedCommunityIds = result);
  }

  String _getLocationDisplayText(GenExperienceState state) {
    // If user has explicitly selected a location, show the formatted name
    if (state.selectedLocationId != null) {
      return _locationDisplayText ?? 'Loading location...';
    }

    // If AI geocoded a location, show its name as a suggestion
    if (state.geocodedLocation != null &&
        state.geocodedLocation!.name.isNotEmpty) {
      return state.geocodedLocation!.name;
    }

    // Otherwise prompt user to set location
    return 'Tap to set location';
  }

  Future<void> _loadLocationDisplayText() async {
    final state = ref.read(genExperienceProvider);

    // Priority 1: If user has explicitly selected a location, load and display it
    if (state.selectedLocationId != null) {
      try {
        final locationRepo = ref.read(locationRepositoryProvider);
        final location = await locationRepo.getLocation(
          state.selectedLocationId!,
        );

        if (!mounted) return;

        setState(() {
          _locationDisplayText = LocationFormatter.formatLocationNameShort(
            location,
          );
        });
        return;
      } catch (e) {
        // If location fetch fails, continue to fallback
      }
    }

    // Priority 2: If we have a geocoded location from AI, format and display it
    if (state.geocodedLocation != null) {
      if (!mounted) return;
      setState(() {
        _locationDisplayText = _formatGeocodedLocation(state.geocodedLocation!);
      });
      return;
    }

    // Priority 3: Fallback to user's primary residence location
    try {
      final authState = ref.read(authStateProvider);
      if (authState.user?.id == null) return;

      final userRepository = ref.read(userRepositoryProvider);
      final user = await userRepository.get(authState.user!.id);

      if (!mounted) return;

      if (user.primaryResidenceLocationId.isNotEmpty) {
        final locationRepo = ref.read(locationRepositoryProvider);
        final location = await locationRepo.getLocation(
          user.primaryResidenceLocationId,
        );

        if (!mounted) return;

        setState(() {
          _locationDisplayText = LocationFormatter.formatLocationNameShort(
            location,
          );
        });
      }
    } catch (e) {
      // If primary location fetch fails, leave as "Tap to set location"
    }
  }

  /// Formats a GeocodedLocation (proto) into a human-readable string.
  ///
  /// Uses the same priority hierarchy as LocationFormatter.formatLocationNameShort:
  /// 1. Place name only
  /// 2. First street address line only
  /// 3. City/locality only
  /// 4. Region code
  /// 5. "Unknown" fallback
  String _formatGeocodedLocation(GeocodedLocation location) {
    if (location.name.isNotEmpty) return location.name;
    if (location.addressLines.isNotEmpty) return location.addressLines.first;
    if (location.locality.isNotEmpty) return location.locality;
    if (location.regionCode.isNotEmpty) return location.regionCode;
    return 'Unknown';
  }

  Future<void> _showLocationPicker() async {
    final state = ref.read(genExperienceProvider);

    final result = await LocationPickerHelper.showLocationPicker(
      context: context,
      ref: ref,
      locationId: state.selectedLocationId,
      // Only pass geocoded location if user hasn't selected a location yet
      geocodedLocation: state.selectedLocationId == null
          ? state.geocodedLocation
          : null,
      allowNonOwnerEdit: true,
    );

    // Handle result after modal is fully closed
    if (result != null && mounted) {
      ref.read(genExperienceProvider.notifier).updateLocation(result);
      await _loadLocationDisplayText();
    }
  }

  Widget _buildActionButton(BuildContext context, GenExperienceState state) {
    final isPast = _isTimeInPast(state.selectedTime);
    // Disable the create button until the streaming generation's terminal
    // event lands — tapping before `final` would save an experience missing
    // media, time, or other fields that haven't arrived yet.
    final isStreaming = state.currentStep == GenExperienceStep.generating;
    final disabled = state.isLoading || isStreaming;
    return SizedBox(
      width: double.infinity,
      child: ContentActionButton(
        label: isPast
            ? context.l10n.experiencePreviewMarkCompleted
            : context.l10n.experiencePreviewCreate,
        backgroundColor: AppColors.primary(context),
        isLoading: state.isLoading || isStreaming,
        onPressed: disabled ? null : _handleCreateExperience,
      ),
    );
  }
}

/// _BackgroundMediaImage rebuilds when media ID changes and delegates to
/// BackgroundMediaImage for proper URL-stable caching.
class _BackgroundMediaImage extends ConsumerWidget {
  final String mediaId;

  const _BackgroundMediaImage({super.key, required this.mediaId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return BackgroundMediaImage(
      mediaId: mediaId,
      getMediaUrl: () => ref.read(mediaRepositoryProvider).getMediaUrl(mediaId),
    );
  }
}
