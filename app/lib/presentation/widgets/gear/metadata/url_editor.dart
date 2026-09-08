import 'package:flutter/material.dart';
import 'package:ripls/core/errors/rpc_error_handler.dart' show ServiceException;
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/gear_metadata_formatter.dart'
    show GearMetadataFormatter;
import 'package:ripls/data/gen/ripls/api/common.pb.dart'
    show MaterialCategory;
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show DetectedGearItem;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:url_launcher/url_launcher.dart';

/// Callback invoked when autofill completes successfully with detected gear
/// fields to merge into the editor state.
typedef AutoFillApplied = void Function({
  String? brand,
  String? model,
  String? category,
  MaterialCategory? materialCategory,
  double? estimatedValueUsd,
  double? weightGrams,
});

/// Displays or edits the product-page URL field and triggers auto-fill.
///
/// In read-only mode renders a tappable link card. In edit mode renders an
/// inline URL text field with an auto-fill button. The [onAutoFill] callback
/// is provided by the parent sheet and never null when [readOnly] is false.
class UrlEditor extends StatefulWidget {
  final TextEditingController urlController;
  final bool readOnly;
  final Future<DetectedGearItem?> Function(String url)? onAutoFill;
  final AutoFillApplied onAutoFillApplied;

  const UrlEditor({
    super.key,
    required this.urlController,
    required this.readOnly,
    required this.onAutoFill,
    required this.onAutoFillApplied,
  });

  @override
  State<UrlEditor> createState() => _UrlEditorState();
}

class _UrlEditorState extends State<UrlEditor> {
  bool _isAutoFilling = false;
  String? _autoFillError;

  Future<void> _handleAutoFill(String url) async {
    if (widget.onAutoFill == null) return;
    setState(() {
      _isAutoFilling = true;
      _autoFillError = null;
    });

    try {
      final detected = await widget.onAutoFill!(url);
      if (!mounted) return;

      if (detected != null) {
        widget.onAutoFillApplied(
          brand: GearMetadataFormatter.isUnknownOrEmpty(detected.brand)
              ? null
              : detected.brand,
          model: GearMetadataFormatter.isUnknownOrEmpty(detected.model)
              ? null
              : detected.model,
          category: GearMetadataFormatter.isUnknownOrEmpty(detected.category)
              ? null
              : detected.category,
          materialCategory:
              detected.materialCategory !=
                      MaterialCategory.MATERIAL_CATEGORY_UNSPECIFIED
                  ? detected.materialCategory
                  : null,
          estimatedValueUsd:
              detected.hasValueEstimate() &&
                      detected.valueEstimate.estimatedValueUsd > 0
                  ? detected.valueEstimate.estimatedValueUsd
                  : null,
          weightGrams:
              detected.hasWeightGrams() &&
                      detected.weightGrams.hasMean() &&
                      detected.weightGrams.mean > 0
                  ? detected.weightGrams.mean
                  : null,
        );
        setState(() => _isAutoFilling = false);
      } else {
        setState(() {
          _isAutoFilling = false;
          _autoFillError = "Couldn't read product details from that page";
        });
      }
    } catch (e) {
      if (!mounted) return;
      final message = e is ServiceException && e.message.contains('permission')
          ? "This site doesn't allow reading from their page"
          : "Couldn't read that page — try a different URL or fill in manually";
      setState(() {
        _isAutoFilling = false;
        _autoFillError = message;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final urlText = widget.urlController.text.trim();
    final hasUrl = urlText.isNotEmpty;

    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 16),
      child: widget.readOnly
          ? _ReadOnlyUrlCard(url: urlText)
          : _EditableUrlSection(
              urlController: widget.urlController,
              urlText: urlText,
              hasUrl: hasUrl,
              isAutoFilling: _isAutoFilling,
              autoFillError: _autoFillError,
              hasAutoFillCallback: widget.onAutoFill != null,
              onAutoFillTap: () => _handleAutoFill(urlText),
              onClearError: () {
                if (mounted && _autoFillError != null) {
                  setState(() => _autoFillError = null);
                }
              },
            ),
    );
  }
}

class _ReadOnlyUrlCard extends StatelessWidget {
  final String url;

  const _ReadOnlyUrlCard({required this.url});

  @override
  Widget build(BuildContext context) {
    final uri = Uri.tryParse(url);
    final displayUrl = url.replaceAll(RegExp(r'^https?://(www\.)?'), '');

    return Tappable(
      semanticsLabel: context.l10n.a11yGearOpenProductPage,
      isLink: true,
      onTap: () async {
        if (uri != null) {
          try {
            await launchUrl(uri, mode: LaunchMode.externalApplication);
          } catch (_) {}
        }
      },
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
        decoration: BoxDecoration(
          color: AppColors.modalInsetCardBg,
          borderRadius: BorderRadius.circular(24),
          border: Border.all(color: AppColors.modalInsetCardBorder),
        ),
        child: Row(
          children: [
            Icon(Icons.open_in_new, size: 14, color: AppColors.primary(context)),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                displayUrl,
                style: TextStyle(
                  fontSize: 12,
                  color: AppColors.primary(context),
                ),
                overflow: TextOverflow.ellipsis,
              ),
            ),
            Text(
              ' ↗',
              style: TextStyle(
                fontSize: 12,
                color: AppColors.modalTextMuted,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _EditableUrlSection extends StatelessWidget {
  final TextEditingController urlController;
  final String urlText;
  final bool hasUrl;
  final bool isAutoFilling;
  final String? autoFillError;
  final bool hasAutoFillCallback;
  final VoidCallback onAutoFillTap;
  final VoidCallback onClearError;

  const _EditableUrlSection({
    required this.urlController,
    required this.urlText,
    required this.hasUrl,
    required this.isAutoFilling,
    required this.autoFillError,
    required this.hasAutoFillCallback,
    required this.onAutoFillTap,
    required this.onClearError,
  });

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text.rich(
          TextSpan(
            text: 'Product page ',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: AppColors.modalTextSecondary,
            ),
            children: [
              TextSpan(
                text: '(optional)',
                style: TextStyle(
                  fontWeight: FontWeight.normal,
                  color: AppColors.modalTextMuted,
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 10),
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
          decoration: BoxDecoration(
            color: AppColors.modalInsetCardBg,
            borderRadius: BorderRadius.circular(24),
            border: Border.all(color: AppColors.modalInsetCardBorder),
          ),
          child: Row(
            children: [
              Icon(Icons.open_in_new,
                  size: 13, color: AppColors.primary(context)),
              const SizedBox(width: 8),
              Expanded(
                child: TextField(
                  controller: urlController,
                  keyboardType: TextInputType.url,
                  textInputAction: TextInputAction.done,
                  onChanged: (_) => onClearError(),
                  decoration: InputDecoration(
                    hintText: 'https://...',
                    hintStyle: TextStyle(
                      fontSize: 12,
                      color: AppColors.modalTextMuted
                          .withValues(alpha: 0.7),
                    ),
                    filled: true,
                    fillColor: Colors.transparent,
                    border: InputBorder.none,
                    enabledBorder: InputBorder.none,
                    focusedBorder: InputBorder.none,
                    isDense: true,
                    contentPadding: EdgeInsets.zero,
                  ),
                  style: TextStyle(
                    fontSize: 12,
                    color: AppColors.modalTextPrimary,
                  ),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: 10),
        if (autoFillError != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 8),
            child: Text(
              autoFillError!,
              style: const TextStyle(
                fontSize: 12,
                color: Colors.red,
                height: 1.4,
              ),
            ),
          ),
        if (hasUrl && hasAutoFillCallback)
          _AutoFillButton(
            isAutoFilling: isAutoFilling,
            onTap: onAutoFillTap,
          )
        else if (!hasUrl)
          Text(
            "Add a link to auto-fill specs, or leave blank — we'll estimate from the photo.",
            style: TextStyle(
              fontSize: 12,
              color: AppColors.modalTextMuted,
              height: 1.4,
            ),
          ),
      ],
    );
  }
}

class _AutoFillButton extends StatelessWidget {
  final bool isAutoFilling;
  final VoidCallback onTap;

  const _AutoFillButton({required this.isAutoFilling, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: context.l10n.a11yGearAutoFillFromUrl,
      onTap: isAutoFilling ? null : onTap,
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (isAutoFilling)
            SizedBox(
              width: 14,
              height: 14,
              child: CircularProgressIndicator(
                strokeWidth: 1.5,
                valueColor:
                    AlwaysStoppedAnimation(AppColors.primary(context)),
              ),
            )
          else
            Icon(
              Icons.remove_red_eye_outlined,
              size: 15,
              color: AppColors.primary(context),
            ),
          const SizedBox(width: 6),
          Text(
            'Auto-fill from this page',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: isAutoFilling
                  ? AppColors.modalTextMuted
                  : AppColors.primary(context),
            ),
          ),
        ],
      ),
    );
  }
}
