import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';

/// CarouselAddMediaPage renders the "upload" page shown at the end of the
/// carousel when adding media is permitted.
class CarouselAddMediaPage extends StatelessWidget {
  final VoidCallback onUpload;

  const CarouselAddMediaPage({super.key, required this.onUpload});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Container(
            padding: const EdgeInsets.all(24),
            decoration: BoxDecoration(
              color: Colors.white.withValues(alpha: 0.1),
              shape: BoxShape.circle,
              border: Border.all(
                color: Colors.white.withValues(alpha: 0.3),
                width: 2,
              ),
            ),
            child: const Icon(
              Icons.add_photo_alternate,
              size: 64,
              color: Colors.white,
            ),
          ),
          const SizedBox(height: 24),
          ElevatedButton.icon(
            onPressed: onUpload,
            icon: const Icon(Icons.upload),
            label: Text(context.l10n.commonUpload),
            style: ElevatedButton.styleFrom(
              padding:
                  const EdgeInsets.symmetric(horizontal: 32, vertical: 16),
              backgroundColor: Colors.white,
              foregroundColor: Colors.black,
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
        ],
      ),
    );
  }
}
