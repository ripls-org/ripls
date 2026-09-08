import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers.dart';

/// ContentAvatar displays a user's avatar with async media loading.
///
/// This widget is used across content views (experiences, requests, gear) to show
/// user avatars in a standardized way with consistent fallback behavior.
///
/// Features:
/// - Async avatar image loading via MediaRepository
/// - Fallback to user initials when no image available
/// - Customizable size and background color
/// - Gradient background for fallback avatars
/// - ClipOval for circular display
///
/// Usage:
/// ```dart
/// ContentAvatar(
///   user: rsvp.user,
///   size: 36,
///   backgroundColor: AppColors.experienceSageGreen,
/// )
/// ```
class ContentAvatar extends ConsumerWidget {
  /// The user whose avatar to display
  final User user;

  /// Size of the avatar (width and height)
  final double size;

  /// Background color for the avatar (used in fallback and +N indicators)
  final Color backgroundColor;

  const ContentAvatar({
    super.key,
    required this.user,
    this.size = 36,
    this.backgroundColor = const Color(0xFF7A9B8C), // Default sage green
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return SizedBox(
      width: size,
      height: size,
      child: ClipOval(
        child: user.mediaId.isNotEmpty
            ? FutureBuilder<String?>(
                future: ref
                    .read(mediaRepositoryProvider)
                    .getMediaUrl(user.mediaId)
                    .then((mediaUrl) => mediaUrl.url),
                builder: (context, snapshot) {
                  if (snapshot.hasData && snapshot.data != null) {
                    return CachedMediaImage(
                      // Decorative; the surrounding card carries the semantic label.
                      semanticsLabel: null,imageUrl: snapshot.data!,
                      cacheKey: user.mediaId,
                      width: size,
                      height: size,
                      fit: BoxFit.cover,
                      errorWidget: _buildFallbackAvatar(),
                      placeholder: _buildFallbackAvatar(),
                    );
                  }
                  return _buildFallbackAvatar();
                },
              )
            : _buildFallbackAvatar(),
      ),
    );
  }

  Widget _buildFallbackAvatar() {
    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [backgroundColor, backgroundColor.withValues(alpha: 0.7)],
        ),
      ),
      child: Center(
        child: Text(
          user.name.isNotEmpty ? user.name[0].toUpperCase() : '?',
          style: TextStyle(
            color: Colors.white,
            fontSize: size * 0.4, // Scale font size with avatar size
            fontWeight: FontWeight.bold,
          ),
        ),
      ),
    );
  }
}
