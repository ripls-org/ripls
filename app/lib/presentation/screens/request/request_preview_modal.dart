import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:keyboard_actions/keyboard_actions.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/glass_tokens.gen.dart';
import 'package:ripls/core/utils/location_formatter.dart';
import 'package:ripls/core/utils/location_picker_helper.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/gen_request_view_model.dart';
import 'package:ripls/presentation/viewmodels/request_compose_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/content/content_action_button.dart';
import 'package:ripls/presentation/widgets/content/content_editable_field.dart';
import 'package:ripls/presentation/widgets/content/content_editing_mixin.dart';
import 'package:ripls/presentation/widgets/content/content_error_banner.dart';
import 'package:ripls/presentation/widgets/creation/seed_needs_field.dart';
import 'package:ripls/presentation/widgets/creation/streaming_skeleton.dart';
import 'package:ripls/presentation/widgets/keyboard_actions_config.dart';
import 'package:ripls/presentation/widgets/keyboard_dismiss_wrapper.dart';
import 'package:ripls/presentation/widgets/media/background_media_image.dart';
import 'package:ripls/presentation/widgets/media/media_picker_button.dart';
import 'package:ripls/presentation/widgets/media/media_picker_dialog.dart';
import 'package:ripls/presentation/widgets/request/compose/request_compose_section.dart';
import 'package:ripls/presentation/widgets/sharing/community_selection_sheet.dart';
import 'package:ripls/services/providers.dart';

final _log = Logger('RequestPreviewModal');

/// RequestPreviewModal shows a preview of AI-generated request suggestions
/// in a full-screen modal overlay. Users can edit the suggestions before creating
/// the request in the database.
class RequestPreviewModal extends ConsumerStatefulWidget {
  const RequestPreviewModal({super.key});

  @override
  ConsumerState<RequestPreviewModal> createState() =>
      _RequestPreviewModalState();
}

class _RequestPreviewModalState extends ConsumerState<RequestPreviewModal>
    with ContentEditingMixin {
  bool _isCreating = false;
  String? _errorMessage;
  List<String> _selectedCommunityIds = [];
  final _titleFocusNode = FocusNode();
  final _descriptionFocusNode = FocusNode();

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
      final state = ref.read(genRequestProvider);

      _log.info('🎬 RequestPreviewModal initState');
      _log.info('📍 State on init - selectedLocationId: ${state.selectedLocationId}');
      _log.info('📍 State on init - geocodedLocation: ${state.geocodedLocation?.name ?? "null"}');
      _log.info('📍 State on init - generatedLocationId: ${state.generatedLocationId}');

      // Set initial values in editing buffer
      initializeEditing(
        initialTitle: state.generatedTitle ?? '',
        initialDescription: state.generatedDescription ?? '',
      );

      // Update ViewModel with initial values
      ref.read(genRequestProvider.notifier).updateTitle(editingTitle);
      ref
          .read(genRequestProvider.notifier)
          .updateDescription(editingDescription);

      // Resolve the current location (if any) into ViewModel state so the
      // preview row can render its formatted name.
      ref.read(genRequestProvider.notifier).ensureLocationResolved();

      // Initialize community selection from enabled communities
      _selectedCommunityIds =
          ref.read(communitiesProvider).communityIds;
    });
  }

  void _showMediaPickerDialog() {
    final state = ref.read(genRequestProvider);
    final hasMedia = state.newMediaId != null || state.generatedMediaId != null;

    MediaPickerDialog.show(
      context: context,
      hasMedia: hasMedia,
      onVideoTap: () {}, // Requests don't support video
      onPhotoTap: () => _handleMediaPick(
        ref.read(genRequestProvider.notifier).pickAndUploadMultipleImagesFromGallery,
      ),
      onCameraTap: () => _handleMediaPick(
        ref.read(genRequestProvider.notifier).pickAndUploadImageFromCamera,
      ),
    );
  }

  Future<void> _handleMediaPick(Future<void> Function() pickFunction) async {
    try {
      await pickFunction();
      if (!mounted) return;
      final failedCount =
          ref.read(genRequestProvider).batchUploadFailedCount;
      if (failedCount != null && failedCount > 0) {
        ToastHelper.showError(
          context,
          context.l10n.mediaBatchUploadPartialFailure(failedCount),
        );
      } else {
        ToastHelper.showSuccess(context, 'Media uploaded successfully!');
      }
    } catch (e) {
      if (mounted) {
        ToastHelper.showError(context, 'Failed to upload media: $e');
      }
    }
  }

  Future<void> _showLocationPicker() async {
    final state = ref.read(genRequestProvider);

    _log.info('🗺️ Opening LocationPicker');
    _log.info('📍 Passing locationId: ${state.selectedLocationId}');
    _log.info('📍 Passing geocodedLocation: ${state.selectedLocationId == null ? state.geocodedLocation?.name ?? "null" : "null (selectedLocationId exists)"}');

    final result = await LocationPickerHelper.showLocationPicker(
      context: context,
      ref: ref,
      locationId: state.selectedLocationId,
      // Only pass geocoded location if user hasn't selected a location yet
      geocodedLocation: state.selectedLocationId == null ? state.geocodedLocation : null,
      allowNonOwnerEdit: true,
    );

    // Handle result after modal is fully closed
    if (result != null && mounted) {
      _log.info('✅ LocationPicker returned result: $result');
      await ref.read(genRequestProvider.notifier).updateLocation(result);
    } else {
      _log.info('❌ LocationPicker returned null (user cancelled)');
    }
  }

  /// Step 2: Create request with final values (saves to database)
  Future<void> _handleCreate() async {
    final title = editingTitle.trim();
    final description = editingDescription.trim();

    if (title.isEmpty) {
      setState(() {
        _errorMessage = 'Request title is required';
      });
      return;
    }

    if (description.isEmpty) {
      setState(() {
        _errorMessage = 'Request description is required';
      });
      return;
    }

    if (_selectedCommunityIds.isEmpty) {
      setState(() {
        _errorMessage = context.l10n.communitySelectRequired;
      });
      return;
    }

    setState(() {
      _isCreating = true;
      _errorMessage = null;
    });

    try {
      // Update title and description in ViewModel
      ref.read(genRequestProvider.notifier).updateTitle(title);
      ref.read(genRequestProvider.notifier).updateDescription(description);

      // Submit the request (media already uploaded by ViewModel if needed)
      await ref
          .read(genRequestProvider.notifier)
          .submitRequest(_selectedCommunityIds);

      if (!mounted) return;

      final finalState = ref.read(genRequestProvider);

      if (finalState.hasError) {
        setState(() {
          _errorMessage = finalState.error == null
              ? 'Failed to post request'
              : RpcErrorHandler.localize(finalState.error!, context.l10n);
          _isCreating = false;
        });
        return;
      }

      if (finalState.isCompleted) {
        final requestId = finalState.createdRequestId;
        if (requestId != null && requestId.isNotEmpty) {
          await _publishComposeDraft(
            requestId: requestId,
            communityId: _selectedCommunityIds.first,
          );
        }
        if (!mounted) return;
        // Return the created request ID
        Navigator.of(context).pop(requestId);
      }
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _errorMessage = e.toString();
        _isCreating = false;
      });
    }
  }

  /// Runs the compose-publish workflow after the parent SubmitRequest
  /// succeeded. Reports partial-success outcomes via a toast (so the
  /// pop still proceeds — the user lands on the published Request and
  /// can complete recovery there).
  Future<void> _publishComposeDraft({
    required String requestId,
    required String communityId,
  }) async {
    final composeState = ref.read(requestComposeProvider);
    if (composeState.pieces.isEmpty) return;

    try {
      final result = await ref
          .read(requestComposeProvider.notifier)
          .publish(requestId: requestId, communityId: communityId);
      if (!mounted) return;
      if (result.failedAdds.isNotEmpty) {
        ToastHelper.showError(
          context,
          context.l10n.composePartialAddError(result.failedAdds.length),
        );
      } else if (result.failedClaims.isNotEmpty) {
        ToastHelper.showError(
          context,
          context.l10n.composePartialClaimError(result.failedClaims.length),
        );
      }
    } catch (e) {
      _log.warning('compose publish failed', e);
      if (!mounted) return;
      ToastHelper.showError(context, e.toString());
    }
  }

  void _handleCancel() {
    Navigator.of(context).pop(); // Returns null (cancelled)
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(genRequestProvider);
    final effectiveMediaId = state.finalMediaId;

    // Streaming generation lands title / description into genRequestProvider
    // after initState has already snapshotted the (empty) buffer. Sync the
    // editing buffer with the incoming values as long as the user hasn't typed
    // anything of their own in the meantime.
    ref.listen<GenRequestState>(genRequestProvider, (prev, next) {
      final incomingTitle = next.generatedTitle;
      if (incomingTitle != null &&
          incomingTitle.isNotEmpty &&
          incomingTitle != editingTitle &&
          editingTitle.isEmpty) {
        updateEditingTitle(incomingTitle);
        ref.read(genRequestProvider.notifier).updateTitle(incomingTitle);
      }
      final incomingDescription = next.generatedDescription;
      if (incomingDescription != null &&
          incomingDescription.isNotEmpty &&
          incomingDescription != editingDescription &&
          editingDescription.isEmpty) {
        updateEditingDescription(incomingDescription);
        ref
            .read(genRequestProvider.notifier)
            .updateDescription(incomingDescription);
      }
    });

    return Dialog.fullscreen(
      child: Scaffold(
        backgroundColor: AppColors.background(context),
        body: KeyboardActions(
          disableScroll: true,
          config: buildKeyboardActionsConfig([_titleFocusNode, _descriptionFocusNode]),
          child: KeyboardDismissWrapper(
            child: Stack(
              children: [
                // Background image if available
                if (effectiveMediaId != null && effectiveMediaId.isNotEmpty)
              BackgroundMediaImage(
                mediaId: effectiveMediaId,
                getMediaUrl: () => ref
                    .read(genRequestProvider.notifier)
                    .getMediaUrl(effectiveMediaId),
              ),

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
                          _buildHeader(context),
                          const Spacer(),

                          // Media picker button
                          Padding(
                            padding: const EdgeInsets.only(bottom: 24),
                            child: MediaPickerButton(
                              hasMedia:
                                  effectiveMediaId != null &&
                                  effectiveMediaId.isNotEmpty,
                              isUploading: state.isUploadingMedia,
                              onTap: _showMediaPickerDialog,
                            ),
                          ),

                          // Editable request details
                          _buildRequestDetails(context),
                          const SizedBox(height: 16),

                          // Location selector
                          _buildLocationSelector(context),
                          const SizedBox(height: 12),

                          // Community selector
                          _buildCommunitySelector(),
                          const SizedBox(height: 24),

                          // Compose section — chips, paste-list, pieces
                          // with inline rename + pre-claim. Each piece
                          // becomes a Need on publish; pre-claimed
                          // pieces are auto-claimed by the requester.
                          RequestComposeSection(
                            suggestions: state.breakdownPieces,
                            onGlass: true,
                          ),
                          const SizedBox(height: 24),

                          // Streaming-side error (AI generation failure)
                          if (state.error != null) ...[
                            ContentErrorBanner(
                              errorMessage: RpcErrorHandler.localize(
                                  state.error!, context.l10n),
                            ),
                            const SizedBox(height: 16),
                          ],

                          // Create-RPC error
                          if (_errorMessage != null) ...[
                            ContentErrorBanner(errorMessage: _errorMessage!),
                            const SizedBox(height: 16),
                          ],

                          // Action buttons
                          _buildActionButtons(context),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
            ),

            // Close button (top-right)
            if (!_isCreating)
              Positioned(
                right: 12,
                top: 12,
                child: SafeArea(
                  child: IconAction(
                    icon: Icons.close,
                    semanticsLabel: context.l10n.a11yClose,
                    onPressed: _handleCancel,
                    color: GlassTokens.textPrimary,
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

  Widget _buildHeader(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Row(
          children: [
            Icon(Icons.auto_awesome, color: GlassTokens.textPrimary, size: 20),
            SizedBox(width: 8),
            Text(
              'Generated Preview',
              style: TextStyle(
                color: GlassTokens.textPrimary,
                fontSize: 14,
                fontWeight: FontWeight.w600,
              ),
            ),
          ],
        ),
        const SizedBox(height: 4),
        Text(
          'Review and edit before posting',
          style: TextStyle(
            color: GlassTokens.textMuted,
            fontSize: 12,
          ),
        ),
      ],
    );
  }

  Future<void> _openCommunityPicker() async {
    final result = await CommunitySelectionSheet.showForDeferred(
      context,
      source: CommunitySelectionSource.requestCreate,
      initialSelection: _selectedCommunityIds,
    );
    if (result != null) setState(() => _selectedCommunityIds = result);
  }

  Widget _buildCommunitySelector() {
    final communities = ref.watch(communitiesProvider).communities;
    final label = _selectedCommunityIds.isEmpty
        ? context.l10n.communitySelectPlaceholder
        : _selectedCommunityIds.length == 1
            ? communities
                  .firstWhere((c) => c.id == _selectedCommunityIds.first)
                  .name
            : context.l10n.communitySelectCount(_selectedCommunityIds.length);

    return Tappable(
      semanticsLabel: context.l10n.a11yReqChooseCommunities,
      onTap: _isCreating ? null : _openCommunityPicker,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: GlassTokens.borderSoft),
        ),
        child: Row(
          children: [
            const Icon(Icons.group_outlined, color: GlassTokens.textMuted, size: 20),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(color: GlassTokens.textPrimary, fontSize: 15),
              ),
            ),
            const Icon(Icons.chevron_right, color: GlassTokens.textMuted, size: 20),
          ],
        ),
      ),
    );
  }

  Widget _buildRequestDetails(BuildContext context) {
    final state = ref.watch(genRequestProvider);
    final isStreaming = state.currentStep == GenRequestStep.generating;
    final titleReady = (state.generatedTitle ?? '').isNotEmpty;
    final descriptionReady = (state.generatedDescription ?? '').isNotEmpty;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        if (isStreaming) ...[
          StreamingGeneratingBanner(label: context.l10n.genStreamingGenerating),
          const SizedBox(height: 12),
        ],
        // Title field using ContentEditableField
        StreamingFieldSlot(
          ready: !isStreaming || titleReady,
          height: 56,
          child: ContentEditableField(
            value: editingTitle,
            label: 'Request Title',
            hintText: 'Request title',
            textFieldKey: const Key('request_title_field'),
            focusNode: _titleFocusNode,
            onChanged: (value) {
              updateEditingTitle(value);
              ref.read(genRequestProvider.notifier).updateTitle(value);
            },
            enabled: !_isCreating,
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
            hintText: 'Request description',
            textFieldKey: const Key('request_description_field'),
            focusNode: _descriptionFocusNode,
            onChanged: (value) {
              updateEditingDescription(value);
              ref.read(genRequestProvider.notifier).updateDescription(value);
            },
            enabled: !_isCreating,
            maxLines: 8,
            minLines: 4,
            useOverlayStyle: true,
          ),
        ),
        const SizedBox(height: 12),
        // The seeded needs (#2702, #2731): the request is born with one
        // claimable need per entry — surfaced editable so extraction is never
        // invisible, and the requester can prune or add before saving. An empty
        // list means the request is born with no needs.
        SeedNeedsField(
          key: const Key('request_seed_needs_field'),
          needs: state.seedNeeds,
          enabled: !_isCreating,
          onChanged: (needs) =>
              ref.read(genRequestProvider.notifier).updateSeedNeeds(needs),
        ),
        const SizedBox(height: 12),
        _buildNeededByRow(context, state),
      ],
    );
  }

  /// Optional "Needed by" date row. When set, the request is placed on this date
  /// in the Home calendar / Up-next agenda. Tapping opens a date picker;
  /// tapping the clear affordance removes the date.
  Widget _buildNeededByRow(BuildContext context, GenRequestState state) {
    final unixSec = state.neededByUnixSec;
    final hasDate = unixSec != null && unixSec > 0;
    final label = hasDate
        ? context.l10n.requestNeededByDate(
            DateTime.fromMillisecondsSinceEpoch(unixSec * 1000))
        : context.l10n.requestNeededByAdd;
    return Tappable(
      semanticsLabel: label,
      onTap: _isCreating ? null : () => _pickNeededBy(context, unixSec),
      inkBorderRadius: BorderRadius.circular(12),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 14),
        decoration: BoxDecoration(
          border: Border.all(color: AppColors.border(context)),
          borderRadius: BorderRadius.circular(12),
        ),
        child: Row(
          children: [
            Icon(Icons.event_outlined,
                size: 18, color: AppColors.textSecondary(context)),
            const SizedBox(width: 10),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  fontSize: 14,
                  color: hasDate
                      ? AppColors.textPrimary(context)
                      : AppColors.textSecondary(context),
                ),
              ),
            ),
            if (hasDate)
              IconAction(
                semanticsLabel: context.l10n.requestNeededByClear,
                icon: Icons.close,
                iconSize: 18,
                onPressed: () =>
                    ref.read(genRequestProvider.notifier).setNeededBy(null),
              ),
          ],
        ),
      ),
    );
  }

  Future<void> _pickNeededBy(BuildContext context, int? currentUnixSec) async {
    final now = DateTime.now();
    final initial = currentUnixSec != null && currentUnixSec > 0
        ? DateTime.fromMillisecondsSinceEpoch(currentUnixSec * 1000)
        : now;
    final picked = await showDatePicker(
      context: context,
      initialDate: initial.isBefore(now) ? now : initial,
      firstDate: now,
      lastDate: now.add(const Duration(days: 365)),
    );
    if (picked == null) return;
    ref
        .read(genRequestProvider.notifier)
        .setNeededBy(picked.millisecondsSinceEpoch ~/ 1000);
  }

  Widget _buildLocationSelector(BuildContext context) {
    final state = ref.watch(genRequestProvider);
    final hasExtractedLocation =
        state.locationQuery != null && state.locationQuery!.isNotEmpty;
    final isStreaming = state.currentStep == GenRequestStep.generating;
    final locationReady = state.geocodedLocation != null ||
        state.selectedLocationId != null ||
        hasExtractedLocation;

    return StreamingFieldSlot(
      ready: !isStreaming || locationReady,
      height: 56,
      child: _buildLocationSelectorInner(context, state, hasExtractedLocation),
    );
  }

  Widget _buildLocationSelectorInner(
    BuildContext context,
    GenRequestState state,
    bool hasExtractedLocation,
  ) {
    return Tappable(
      semanticsLabel: context.l10n.a11yReqViewLocation,
      onTap: _isCreating ? null : _showLocationPicker,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
        decoration: BoxDecoration(
          color: GlassTokens.fillSubtle,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: GlassTokens.borderSoft),
        ),
        child: Row(
          children: [
            Icon(
              Icons.location_on,
              size: 20,
              color: GlassTokens.textSecondary,
            ),
            const SizedBox(width: 8),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Show AI-extracted location query only while geocoding is in progress
                  // (we have a query but no geocoded location yet)
                  if (hasExtractedLocation &&
                      state.geocodedLocation == null &&
                      state.selectedLocationId == null) ...[
                    Text(
                      '📍 ${state.locationQuery}',
                      style: TextStyle(
                        fontSize: 12,
                        color: GlassTokens.textMuted,
                      ),
                    ),
                    const SizedBox(height: 2),
                  ],
                  Text(
                    _getLocationDisplayText(state),
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 14,
                      color: GlassTokens.textSecondary,
                      fontWeight:
                          hasExtractedLocation &&
                              state.selectedLocationId != null
                          ? FontWeight.w500
                          : FontWeight.normal,
                    ),
                  ),
                ],
              ),
            ),
            Icon(
              Icons.chevron_right,
              size: 20,
              color: GlassTokens.textFaint,
            ),
          ],
        ),
      ),
    );
  }

  String _getLocationDisplayText(GenRequestState state) {
    // Prefer the resolved Location proto from state — it's the canonical
    // value populated by GenRequestNotifier after any user pick or initial
    // resolution.
    if (state.resolvedLocation != null) {
      return LocationFormatter.formatLocationNameShort(state.resolvedLocation!);
    }

    // A selection (or AI-generated primary residence) is set but the
    // resolution is still in flight.
    if (state.selectedLocationId != null ||
        state.generatedLocationId != null) {
      return context.l10n.requestPreviewLocationLoading;
    }

    // No selection yet — show the AI's geocoded suggestion if we have one.
    if (state.geocodedLocation != null &&
        state.geocodedLocation!.name.isNotEmpty) {
      return state.geocodedLocation!.name;
    }

    return context.l10n.requestPreviewLocationTapToSet;
  }

  Widget _buildActionButtons(BuildContext context) {
    // Disable the create button until the streaming generation's terminal
    // event lands — tapping before `final` would save a request missing
    // media or other fields that haven't arrived yet.
    final isStreaming =
        ref.watch(genRequestProvider).currentStep == GenRequestStep.generating;
    final disabled = _isCreating || isStreaming;
    return SizedBox(
      width: double.infinity,
      child: ContentActionButton(
        label: 'Share',
        backgroundColor: Colors.blue,
        isLoading: _isCreating || isStreaming,
        onPressed: disabled ? null : _handleCreate,
      ),
    );
  }
}
