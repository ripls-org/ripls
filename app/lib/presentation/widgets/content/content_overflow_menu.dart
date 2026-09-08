import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/attribution_utils.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:url_launcher/url_launcher.dart';

/// MenuItemConfig defines a single menu item in the overflow menu.
class MenuItemConfig {
  /// Label text for the menu item
  final String label;

  /// Icon to display
  final IconData icon;

  /// Callback when item is tapped
  final VoidCallback onTap;

  const MenuItemConfig({
    required this.label,
    required this.icon,
    required this.onTap,
  });
}

/// ContentOverflowMenuConfig defines which standard menu items to show
/// and their callbacks.
class ContentOverflowMenuConfig {
  /// Show edit option (typically owner only)
  final bool showEdit;

  /// Show share link option
  final bool showShare;

  /// Show delete option (typically owner only)
  final bool showDelete;

  /// Show report option (typically non-owners)
  final bool showReport;

  /// Attribution data for background image (displayed inline if present)
  final Attribution? attribution;

  /// Callback when edit is tapped
  final VoidCallback? onEdit;

  /// Callback when share is tapped
  final VoidCallback? onShare;

  /// Callback when delete is tapped
  final VoidCallback? onDelete;

  /// Callback when report is tapped
  final VoidCallback? onReport;

  /// Additional custom menu items
  final List<MenuItemConfig>? customItems;

  const ContentOverflowMenuConfig({
    this.showEdit = false,
    this.showShare = false,
    this.showDelete = false,
    this.showReport = false,
    this.attribution,
    this.onEdit,
    this.onShare,
    this.onDelete,
    this.onReport,
    this.customItems,
  });
}

/// Shows a bottom sheet overflow menu for content actions.
///
/// This provides a consistent overflow menu pattern across all content types
/// (gear, request, experience) with standard actions (edit, share, delete, report)
/// and support for custom items.
Future<void> showContentOverflowMenu({
  required BuildContext context,
  required ContentOverflowMenuConfig config,
}) async {
  return showAccessibleModal(
    context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    barrierColor: AppColors.modalBackdrop,
    builder: (context) => GlassSheet(
      applyMaxHeight: false,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          if (config.showEdit && config.onEdit != null)
            _MenuTile(
              icon: Icons.edit,
              label: context.l10n.commonEdit,
              onTap: () {
                Navigator.pop(context);
                config.onEdit!();
              },
            ),
          if (config.showShare && config.onShare != null)
            _MenuTile(
              icon: Icons.share,
              label: 'Share Link',
              onTap: () {
                Navigator.pop(context);
                config.onShare!();
              },
            ),
          if (config.customItems != null)
            ...config.customItems!.map(
              (item) => _MenuTile(
                icon: item.icon,
                label: item.label,
                onTap: () {
                  Navigator.pop(context);
                  item.onTap();
                },
              ),
            ),
          if (config.showReport && config.onReport != null)
            _MenuTile(
              icon: Icons.flag_outlined,
              label: 'Report',
              onTap: () {
                Navigator.pop(context);
                config.onReport!();
              },
            ),
          if (config.showDelete && config.onDelete != null)
            _MenuTile(
              icon: Icons.delete_outline,
              label: context.l10n.commonDelete,
              onTap: () {
                Navigator.pop(context);
                config.onDelete!();
              },
              isDestructive: true,
            ),
          if (config.attribution != null) ...[
            const SizedBox(height: 4),
            _AttributionSection(attribution: config.attribution!),
          ],
        ],
      ),
    ),
  );
}

class _MenuTile extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onTap;
  final bool isDestructive;

  const _MenuTile({
    required this.icon,
    required this.label,
    required this.onTap,
    this.isDestructive = false,
  });

  @override
  Widget build(BuildContext context) {
    final fg = isDestructive
        ? AppColors.statusWarningOnDark
        : AppColors.modalTextPrimary;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(12),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
        child: Row(
          children: [
            Icon(icon, color: fg, size: 22),
            const SizedBox(width: 14),
            Expanded(
              child: Text(
                label,
                style: TextStyle(
                  color: fg,
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _AttributionSection extends StatelessWidget {
  final Attribution attribution;

  const _AttributionSection({required this.attribution});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 14),
      child: Row(
        children: [
          Icon(
            Icons.camera_alt,
            color: AppColors.modalTextMuted,
            size: 20,
          ),
          const SizedBox(width: 14),
          Expanded(
            child: _AttributionText(attribution: attribution),
          ),
        ],
      ),
    );
  }
}

class _AttributionText extends StatelessWidget {
  final Attribution attribution;

  const _AttributionText({required this.attribution});

  @override
  Widget build(BuildContext context) {
    final style = TextStyle(
      fontSize: 13,
      color: AppColors.modalTextSecondary,
      height: 1.3,
    );
    final providerName = AttributionUtils.getProviderName(attribution);

    return Semantics(
      label: context.l10n
          .a11yContentPhotoBy(attribution.creatorName, providerName),
      child: Wrap(
        children: [
          Text('Photo by ', style: style),
          Tappable(
            semanticsLabel: context.l10n
                .a11yContentAttributionCreator(attribution.creatorName),
            isLink: true,
            onTap: () => _openCreatorProfile(),
            child: Text(
              attribution.creatorName,
              style: style.copyWith(decoration: TextDecoration.underline),
            ),
          ),
          Text(' on ', style: style),
          Tappable(
            semanticsLabel: context.l10n
                .a11yContentAttributionPlatform(providerName),
            isLink: true,
            onTap: () => _openPlatform(),
            child: Text(
              providerName,
              style: style.copyWith(decoration: TextDecoration.underline),
            ),
          ),
        ],
      ),
    );
  }

  Future<void> _openCreatorProfile() async {
    final urlString = AttributionUtils.getCreatorProfileUrl(attribution);
    if (urlString.isEmpty) return;

    final url = Uri.parse(urlString);
    if (await canLaunchUrl(url)) {
      await launchUrl(url, mode: LaunchMode.externalApplication);
    }
  }

  Future<void> _openPlatform() async {
    final urlString = AttributionUtils.getPlatformUrl(attribution);
    if (urlString.isEmpty) return;

    final url = Uri.parse(urlString);
    if (await canLaunchUrl(url)) {
      await launchUrl(url, mode: LaunchMode.externalApplication);
    }
  }
}
