import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/presentation/viewmodels/community_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/app_bar_back_button.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/content_error_banner.dart';
import 'package:ripls/presentation/widgets/keyboard_dismiss_wrapper.dart';
import 'package:ripls/presentation/widgets/swipe_to_close_mixin.dart';
import 'package:ripls/services/community_service.dart';
import 'package:ripls/services/media_service.dart';

import '../../../core/theme/app_colors.dart';
import '../../../core/theme/gen/overlay_tokens.gen.dart';
import '../../../core/utils/toast_helper.dart';

/// CommunityEditScreen allows editing community profile including title, description, and image.
class CommunityEditScreen extends ConsumerStatefulWidget {
  final String communityId;
  final CommunityService communityService;
  final MediaService mediaService;

  const CommunityEditScreen({
    required this.communityId,
    required this.communityService,
    required this.mediaService,
    super.key,
  });

  @override
  ConsumerState<CommunityEditScreen> createState() =>
      _CommunityEditScreenState();
}

class _CommunityEditScreenState extends ConsumerState<CommunityEditScreen>
    with SingleTickerProviderStateMixin, SwipeToCloseMixin {
  final _formKey = GlobalKey<FormState>();
  final _nameController = TextEditingController();
  final _descriptionController = TextEditingController();

  @override
  void initState() {
    super.initState();
    // Initialize the provider with the community ID and enter edit mode
    WidgetsBinding.instance.addPostFrameCallback((_) async {
      await ref.read(communityProvider.notifier).initialize(widget.communityId);
      if (mounted) {
        ref.read(communityProvider.notifier).toggleEditMode();
        // Update text controllers with loaded data
        final state = ref.read(communityProvider);
        _nameController.text = state.communityName ?? '';
        _descriptionController.text = state.communityDescription ?? '';
      }
    });
  }

  @override
  void dispose() {
    _nameController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  Future<void> _handleSave() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    try {
      await ref
          .read(communityProvider.notifier)
          .saveChanges(
            name: _nameController.text,
            description: _descriptionController.text,
          );
      if (!mounted) return;

      Navigator.of(context).pop(true); // Return true to indicate success
    } catch (e) {
      if (!mounted) return;
      ToastHelper.showError(context, context.l10n.communityEditUpdateFailed);
    }
  }

  void _showImageSourceOptions() {
    showAccessibleModal(context,
      backgroundColor: Colors.transparent,
      builder: (context) => _buildImageSourceSheet(),
    );
  }

  Widget _buildImageSourceSheet() {
    return Container(
      decoration: BoxDecoration(
        color: AppColors.cardBackground(context),
        borderRadius: const BorderRadius.only(
          topLeft: Radius.circular(20),
          topRight: Radius.circular(20),
        ),
      ),
      child: SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.photo_library),
              title: Text(context.l10n.communityEditChooseFromGallery),
              onTap: () {
                Navigator.of(context).pop();
                ref.read(communityProvider.notifier).pickImageFromGallery();
              },
            ),
            ListTile(
              leading: const Icon(Icons.camera_alt),
              title: Text(context.l10n.communityEditTakePhoto),
              onTap: () {
                Navigator.of(context).pop();
                ref.read(communityProvider.notifier).pickImageFromCamera();
              },
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }

  Widget _buildImageSection() {
    final state = ref.watch(communityProvider);
    return Tappable(
      semanticsLabel: context.l10n.a11yEdit,
      onTap: state.isSaving ? null : _showImageSourceOptions,
      child: Container(
        height: 200,
        width: double.infinity,
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
            color: AppColors.textTertiary(context).withAlpha(50),
            width: 1,
          ),
        ),
        child: _buildImageContent(),
      ),
    );
  }

  Widget _buildImageContent() {
    final state = ref.watch(communityProvider);

    if (state.isUploadingMedia) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            CircularProgressIndicator(color: AppColors.primary(context)),
            const SizedBox(height: 16),
            Text(
              context.l10n.communityEditUploadingImage,
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 14,
              ),
            ),
          ],
        ),
      );
    }

    if (state.mediaPath != null && !state.isVideo) {
      return Stack(
        children: [
          ClipRRect(
            borderRadius: BorderRadius.circular(12),
            child: CachedMediaImage(
              // Decorative; the surrounding card carries the semantic label.
              semanticsLabel: null,imageUrl: state.mediaPath!,
              cacheKey: ImageCacheKeys.full(state.mediaId),
              width: double.infinity,
              height: double.infinity,
              fit: BoxFit.cover,
              errorWidget: _buildPlaceholder(),
            ),
          ),
          Positioned(
            top: 8,
            right: 8,
            child: Container(
              decoration: BoxDecoration(
                color: Colors.black.withAlpha(128),
                shape: BoxShape.circle,
              ),
              child: IconAction(
                icon: Icons.edit,
                semanticsLabel: context.l10n.a11yEdit,
                color: OverlayTokens.textPrimary,
                iconSize: 20,
                onPressed: state.isSaving ? null : _showImageSourceOptions,
              ),
            ),
          ),
        ],
      );
    }

    return _buildPlaceholder();
  }

  Widget _buildPlaceholder() {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Icon(
          Icons.add_photo_alternate,
          size: 64,
          color: AppColors.textTertiary(context),
        ),
        const SizedBox(height: 12),
        Text(
          context.l10n.communityEditTapToAddImage,
          style: TextStyle(
            color: AppColors.textSecondary(context),
            fontSize: 14,
          ),
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(communityProvider);

    return buildSwipeableScaffold(
      backgroundColor: AppColors.background(context),
      appBar: AppBar(
        backgroundColor: AppColors.background(context),
        title: Text(
          context.l10n.communityEditTitle,
          style: TextStyle(color: AppColors.textPrimary(context)),
        ),
        leading: AppBarBackButton(
          onPressed: state.isSaving
              ? null
              : () {
                  ref.read(communityProvider.notifier).cancelEdit();
                  handleClose();
                },
        ),
        actions: [
          if (state.isSaving)
            Center(
              child: Padding(
                padding: const EdgeInsets.only(right: 16),
                child: SizedBox(
                  height: 20,
                  width: 20,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: AppColors.textPrimary(context),
                  ),
                ),
              ),
            )
          else
            TextButton(
              onPressed: _handleSave,
              child: Text(
                context.l10n.commonSave,
                style: TextStyle(
                  color: AppColors.primary(context),
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
        ],
      ),
      body: KeyboardDismissWrapper(
        child: state.isLoading
            ? Center(
                child: CircularProgressIndicator(
                  color: AppColors.primary(context),
                ),
              )
            : SingleChildScrollView(
              padding: const EdgeInsets.all(24),
              child: Form(
                key: _formKey,
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    _buildImageSection(),
                    const SizedBox(height: 24),
                    TextFormField(
                      controller: _nameController,
                      decoration: InputDecoration(
                        labelText: context.l10n.communityEditNameLabel,
                        labelStyle: TextStyle(
                          color: AppColors.textSecondary(context),
                        ),
                        filled: true,
                        fillColor: AppColors.surface(context),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: BorderSide.none,
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: BorderSide.none,
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: BorderSide(
                            color: AppColors.primary(context),
                            width: 2,
                          ),
                        ),
                      ),
                      style: TextStyle(color: AppColors.textPrimary(context)),
                      textInputAction: TextInputAction.next,
                      enabled: !state.isSaving,
                      validator: (value) {
                        if (value == null || value.trim().isEmpty) {
                          return context.l10n.communityEditNameValidation;
                        }
                        return null;
                      },
                    ),
                    const SizedBox(height: 16),
                    TextFormField(
                      controller: _descriptionController,
                      decoration: InputDecoration(
                        labelText: context.l10n.commonDescription,
                        labelStyle: TextStyle(
                          color: AppColors.textSecondary(context),
                        ),
                        filled: true,
                        fillColor: AppColors.surface(context),
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: BorderSide.none,
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: BorderSide.none,
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(12),
                          borderSide: BorderSide(
                            color: AppColors.primary(context),
                            width: 2,
                          ),
                        ),
                        alignLabelWithHint: true,
                      ),
                      style: TextStyle(color: AppColors.textPrimary(context)),
                      textInputAction: TextInputAction.done,
                      maxLines: 5,
                      enabled: !state.isSaving,
                      validator: (value) {
                        if (value == null || value.trim().isEmpty) {
                          return context.l10n.communityEditDescriptionValidation;
                        }
                        return null;
                      },
                    ),
                    if (state.hasError) ...[
                      const SizedBox(height: 16),
                      ContentErrorBanner(
                        errorMessage:
                            RpcErrorHandler.localize(state.error!, context.l10n),
                      ),
                    ],
                  ],
                ),
              ),
            ),
        ),
    );
  }
}
