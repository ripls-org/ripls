import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// MediaPickerButton displays a circular button for selecting or replacing media.
/// Shows a loading indicator when media is being uploaded.
class MediaPickerButton extends StatelessWidget {
  final bool hasMedia;
  final bool isUploading;
  final VoidCallback? onTap;

  const MediaPickerButton({
    super.key,
    required this.hasMedia,
    required this.isUploading,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final labelText = hasMedia
        ? context.l10n.mediaPickerButtonReplace
        : context.l10n.mediaPickerButtonSelect;
    final icon = hasMedia ? Icons.edit : Icons.add_photo_alternate;
    final semanticsLabel = hasMedia
        ? context.l10n.a11yMediaReplaceBackground
        : context.l10n.a11yMediaSelectMedia;

    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Tappable(
            semanticsLabel: semanticsLabel,
            onTap: isUploading ? null : onTap,
            child: Container(
              width: 80,
              height: 80,
              decoration: BoxDecoration(
                color: Colors.white.withValues(alpha: 0.3),
                shape: BoxShape.circle,
                border: Border.all(
                  color: Colors.white.withValues(alpha: 0.3),
                  width: 2,
                ),
              ),
              child: isUploading
                  ? const CircularProgressIndicator(
                      color: Colors.white,
                      strokeWidth: 2,
                    )
                  : Icon(
                      icon,
                      color: Colors.white,
                      size: 40,
                    ),
            ),
          ),
          const SizedBox(height: 16),
          Text(
            labelText,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
              color: Colors.white,
              fontSize: 16,
              fontWeight: FontWeight.w500,
            ),
          ),
        ],
      ),
    );
  }
}
