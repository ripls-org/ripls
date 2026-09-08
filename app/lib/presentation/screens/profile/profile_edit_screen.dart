import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/gen/overlay_tokens.gen.dart';
import 'package:ripls/core/utils/image_cache_keys.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/presentation/screens/auth/phone_auth_screen.dart';
import 'package:ripls/presentation/viewmodels/user_profile_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/icon_action.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/keyboard_dismiss_wrapper.dart';
import 'package:ripls/services/providers.dart';

/// ProfileEditScreen allows users to edit their profile information.
///
/// Provides form fields for editing name and description.
/// Changes are saved via UserProfileViewModel.saveUserProfile().
class ProfileEditScreen extends ConsumerStatefulWidget {
  const ProfileEditScreen({super.key, required this.userId});

  final String userId;

  @override
  ConsumerState<ProfileEditScreen> createState() => _ProfileEditScreenState();
}

class _ProfileEditScreenState extends ConsumerState<ProfileEditScreen> {
  final _formKey = GlobalKey<FormState>();
  late TextEditingController _nameController;
  late TextEditingController _descriptionController;
  bool _hasChanges = false;

  /// The account's verified phone number, or null/empty if none is attached.
  /// Read from a self GetUser fetch (the only place phone_number is exposed);
  /// drives the add-vs-show state of the phone row.
  String? _phoneNumber;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController();
    _descriptionController = TextEditingController();

    // Initialize ViewModel and load user data
    WidgetsBinding.instance.addPostFrameCallback((_) {
      _initializeViewModel();
    });
  }

  @override
  void dispose() {
    _nameController.dispose();
    _descriptionController.dispose();
    super.dispose();
  }

  Future<void> _initializeViewModel() async {
    final notifier = ref.read(userProfileViewModelProvider.notifier);

    // Initialize the ViewModel with the userId if not already initialized
    await notifier.initialize(widget.userId);

    // Load user data into form fields
    _loadUserData();
    await _loadPhoneNumber();
  }

  /// Reads the current phone number from the cached self GetUser (the only
  /// response that carries phone_number; the viewmodel's profile load has
  /// usually warmed this cache already). Best-effort: on failure the row stays
  /// in its "Add phone number" state.
  Future<void> _loadPhoneNumber() async {
    try {
      final user = await ref.read(userRepositoryProvider).get(widget.userId);
      if (mounted) {
        setState(() => _phoneNumber = user.phoneNumber);
      }
    } catch (_) {
      // Leave _phoneNumber null; the row falls back to the add affordance.
    }
  }

  Future<void> _onAddPhoneTapped() async {
    // Imperative push (not a GoRouter route) is deliberate: PhoneAuthScreen
    // must sit above GoRouter so it survives the reCAPTCHA callback redirect
    // during web phone auth (see the router's redirect note). pushWithSlide
    // is the sanctioned helper for that pattern.
    final newPhone = await NavigationHelpers.pushWithSlide<String>(
      context: context,
      screen: const PhoneAuthScreen(mode: PhoneAuthMode.attach),
      routeName: 'add_phone',
    );
    if (newPhone != null && newPhone.isNotEmpty && mounted) {
      setState(() => _phoneNumber = newPhone);
    }
  }

  void _loadUserData() {
    final vmState = ref.read(userProfileViewModelProvider);
    final user = vmState.user;

    if (user != null) {
      _nameController.text = user.name;
      _descriptionController.text = user.description;
    }
  }

  void _onFieldChanged() {
    setState(() {
      _hasChanges = true;
    });
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) {
      return;
    }

    try {
      await ref.read(userProfileViewModelProvider.notifier).saveUserProfile(
            name: _nameController.text.trim(),
            description: _descriptionController.text.trim(),
          );

      if (mounted) {
        Navigator.of(context).pop(true); // Return true to indicate success
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Failed to save profile: ${e.toString()}'),
            backgroundColor: Colors.red,
          ),
        );
      }
    }
  }

  Future<bool> _onWillPop() async {
    if (!_hasChanges) {
      return true;
    }

    final shouldPop = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Discard changes?'),
        content: const Text(
          'You have unsaved changes. Are you sure you want to discard them?',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: Text(context.l10n.commonCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Discard'),
          ),
        ],
      ),
    );

    return shouldPop ?? false;
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
              title: const Text('Choose from Gallery'),
              onTap: () {
                Navigator.of(context).pop();
                ref.read(userProfileViewModelProvider.notifier).pickImageFromGallery();
                setState(() {
                  _hasChanges = true;
                });
              },
            ),
            ListTile(
              leading: const Icon(Icons.camera_alt),
              title: const Text('Take a Photo'),
              onTap: () {
                Navigator.of(context).pop();
                ref.read(userProfileViewModelProvider.notifier).pickImageFromCamera();
                setState(() {
                  _hasChanges = true;
                });
              },
            ),
            const SizedBox(height: 8),
          ],
        ),
      ),
    );
  }

  Widget _buildImageSection() {
    final vmState = ref.watch(userProfileViewModelProvider);
    return Tappable(
      semanticsLabel: context.l10n.a11yEdit,
      onTap: vmState.isUploadingMedia ? null : _showImageSourceOptions,
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
    final vmState = ref.watch(userProfileViewModelProvider);

    if (vmState.isUploadingMedia) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            CircularProgressIndicator(color: AppColors.primary(context)),
            const SizedBox(height: 16),
            Text(
              'Uploading image...',
              style: TextStyle(
                color: AppColors.textSecondary(context),
                fontSize: 14,
              ),
            ),
          ],
        ),
      );
    }

    if (vmState.mediaUrl != null) {
      final user = vmState.user;
      final mediaId = user?.mediaId ?? '';
      return Stack(
        children: [
          ClipRRect(
            borderRadius: BorderRadius.circular(12),
            child: CachedMediaImage(
              // Decorative; the surrounding card carries the semantic label.
              semanticsLabel: null,imageUrl: vmState.mediaUrl!,
              cacheKey: ImageCacheKeys.full(mediaId),
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
                color: OverlayTokens.scrimTop,
                shape: BoxShape.circle,
              ),
              child: IconAction(
                icon: Icons.edit,
                semanticsLabel: context.l10n.a11yEdit,
                color: OverlayTokens.textPrimary,
                iconSize: 20,
                onPressed: vmState.isUploadingMedia ? null : _showImageSourceOptions,
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
          'Tap to add profile photo',
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
    final vmState = ref.watch(userProfileViewModelProvider);
    final isLoading = vmState.isLoadingUser;
    final isSaving = vmState.isSaving;
    final isUploadingMedia = vmState.isUploadingMedia;

    return PopScope(
      canPop: !_hasChanges,
      onPopInvokedWithResult: (didPop, result) async {
        if (didPop) return;
        final shouldPop = await _onWillPop();
        if (shouldPop && context.mounted) {
          Navigator.of(context).pop();
        }
      },
      child: Scaffold(
        backgroundColor: AppColors.background(context),
        appBar: AppBar(
          backgroundColor: AppColors.appBarBackground(context),
          elevation: 0,
          leading: IconAction(
            icon: Icons.close,
            semanticsLabel: context.l10n.a11yClose,
            color: AppColors.textPrimary(context),
            onPressed: () async {
              final navigator = Navigator.of(context);
              if (_hasChanges) {
                final shouldPop = await _onWillPop();
                if (shouldPop && mounted) {
                  navigator.pop();
                }
              } else {
                navigator.pop();
              }
            },
          ),
          title: Text(
            context.l10n.profileEditTitle,
            style: TextStyle(color: AppColors.textPrimary(context)),
          ),
          actions: [
            if (isSaving)
              const Center(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  ),
                ),
              )
            else
              TextButton(
                onPressed: _hasChanges && !isUploadingMedia ? _save : null,
                child: Text(
                  context.l10n.commonSave,
                  style: TextStyle(
                    color: _hasChanges && !isUploadingMedia
                        ? AppColors.primary(context)
                        : AppColors.textSecondary(context),
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
          ],
        ),
        body: KeyboardDismissWrapper(
          child: isLoading
              ? const Center(child: CircularProgressIndicator())
              : Form(
                  key: _formKey,
                  child: ListView(
                    padding: const EdgeInsets.all(16),
                    children: [
                      _buildImageSection(),
                      const SizedBox(height: 24),
                      _buildNameField(),
                      const SizedBox(height: 24),
                      _buildDescriptionField(),
                      const SizedBox(height: 16),
                      _buildCharacterCount(),
                      const SizedBox(height: 24),
                      _buildPhoneRow(),
                    ],
                  ),
                ),
        ),
      ),
    );
  }

  Widget _buildNameField() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'Name',
          style: Theme.of(context).textTheme.titleSmall?.copyWith(
                color: AppColors.textPrimary(context),
                fontWeight: FontWeight.w600,
              ),
        ),
        const SizedBox(height: 8),
        TextFormField(
          controller: _nameController,
          decoration: InputDecoration(
            hintText: 'Enter your name',
            hintStyle: TextStyle(color: AppColors.textSecondary(context)),
            filled: true,
            fillColor: AppColors.surface(context),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: AppColors.border(context)),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: AppColors.border(context)),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide:
                  BorderSide(color: AppColors.primary(context), width: 2),
            ),
            errorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: const BorderSide(color: Colors.red),
            ),
            focusedErrorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: const BorderSide(color: Colors.red, width: 2),
            ),
          ),
          style: TextStyle(color: AppColors.textPrimary(context)),
          maxLength: 50,
          buildCounter: (context,
                  {required currentLength, required isFocused, maxLength}) =>
              null, // Hide built-in counter
          onChanged: (_) => _onFieldChanged(),
          validator: (value) {
            if (value == null || value.trim().isEmpty) {
              return 'Name is required';
            }
            if (value.trim().length > 50) {
              return 'Name must be 50 characters or less';
            }
            return null;
          },
        ),
      ],
    );
  }

  Widget _buildDescriptionField() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'About',
          style: Theme.of(context).textTheme.titleSmall?.copyWith(
                color: AppColors.textPrimary(context),
                fontWeight: FontWeight.w600,
              ),
        ),
        const SizedBox(height: 8),
        TextFormField(
          controller: _descriptionController,
          decoration: InputDecoration(
            hintText: 'Tell us about yourself...',
            hintStyle: TextStyle(color: AppColors.textSecondary(context)),
            filled: true,
            fillColor: AppColors.surface(context),
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: AppColors.border(context)),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: BorderSide(color: AppColors.border(context)),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide:
                  BorderSide(color: AppColors.primary(context), width: 2),
            ),
            errorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: const BorderSide(color: Colors.red),
            ),
            focusedErrorBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(12),
              borderSide: const BorderSide(color: Colors.red, width: 2),
            ),
            alignLabelWithHint: true,
          ),
          style: TextStyle(color: AppColors.textPrimary(context)),
          maxLength: 500,
          maxLines: 6,
          buildCounter: (context,
                  {required currentLength, required isFocused, maxLength}) =>
              null, // Hide built-in counter
          onChanged: (_) => _onFieldChanged(),
          validator: (value) {
            if (value != null && value.trim().length > 500) {
              return 'Description must be 500 characters or less';
            }
            return null;
          },
        ),
      ],
    );
  }

  /// Low-prominence row to attach a phone number to the account. Shows the
  /// current number with a check when one is set, otherwise an "Add phone
  /// number" affordance. Tapping opens the phone-OTP flow in attach mode.
  Widget _buildPhoneRow() {
    final hasPhone = (_phoneNumber ?? '').isNotEmpty;
    final title = hasPhone
        ? context.l10n.profilePhoneNumber
        : context.l10n.profileAddPhoneNumber;
    return Tappable(
      semanticsLabel: title,
      semanticsIdentifier: 'profile-add-phone',
      onTap: _onAddPhoneTapped,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: BorderRadius.circular(12),
          border: Border.all(color: AppColors.border(context)),
        ),
        child: Row(
          children: [
            Icon(Icons.phone_outlined, color: AppColors.textSecondary(context)),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: TextStyle(color: AppColors.textPrimary(context)),
                  ),
                  if (hasPhone) ...[
                    const SizedBox(height: 2),
                    Text(
                      _phoneNumber!,
                      style: TextStyle(
                        color: AppColors.textSecondary(context),
                        fontSize: 13,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            if (hasPhone)
              Icon(Icons.check_circle,
                  color: AppColors.primary(context), size: 20)
            else
              Icon(Icons.chevron_right, color: AppColors.textTertiary(context)),
          ],
        ),
      ),
    );
  }

  Widget _buildCharacterCount() {
    final nameLength = _nameController.text.length;
    final descLength = _descriptionController.text.length;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 4),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(
            'Name: $nameLength/50',
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: nameLength > 50
                      ? Colors.red
                      : AppColors.textSecondary(context),
                ),
          ),
          Text(
            'About: $descLength/500',
            style: Theme.of(context).textTheme.bodySmall?.copyWith(
                  color: descLength > 500
                      ? Colors.red
                      : AppColors.textSecondary(context),
                ),
          ),
        ],
      ),
    );
  }
}
