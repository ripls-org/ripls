import 'package:flutter/material.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/presentation/widgets/media/media_background.dart';

/// The v11 profile backdrop, photo state: the entity's primary media
/// filling the top ~46% of the screen behind the scrolling identity
/// header, under a scrim that darkens the top for the chrome and
/// dissolves into the dark page so the photo has no hard bottom edge.
/// Position as a full-screen Stack layer *behind* the scroll view.
class ProfileHeroBackdrop extends StatelessWidget {
  const ProfileHeroBackdrop({
    super.key,
    required this.mediaUrl,
    required this.mediaId,
  });

  final String mediaUrl;
  final String? mediaId;

  @override
  Widget build(BuildContext context) {
    const page = AppColors.darkBackground;
    return Column(
      children: [
        Expanded(
          flex: 46,
          child: Stack(
            fit: StackFit.expand,
            children: [
              MediaBackground(mediaPath: mediaUrl, mediaId: mediaId),
              DecoratedBox(
                decoration: BoxDecoration(
                  gradient: LinearGradient(
                    begin: Alignment.topCenter,
                    end: Alignment.bottomCenter,
                    colors: [
                      page.withValues(alpha: 0.5),
                      page.withValues(alpha: 0.22),
                      page.withValues(alpha: 0.72),
                      page,
                    ],
                    stops: const [0.0, 0.22, 0.52, 0.84],
                  ),
                ),
              ),
            ],
          ),
        ),
        const Expanded(flex: 54, child: ColoredBox(color: page)),
      ],
    );
  }
}

/// The v11 no-photo fallback backdrop: an accent-tinted gradient
/// masthead dissolving into the dark page — the default until a real
/// photo exists, with zero layout shift when one arrives.
class ProfileMastheadBackdrop extends StatelessWidget {
  const ProfileMastheadBackdrop({super.key});

  @override
  Widget build(BuildContext context) {
    const page = AppColors.darkBackground;
    return DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color.alphaBlend(
              AppColors.primary(context).withValues(alpha: 0.16),
              page,
            ),
            page,
          ],
          stops: const [0.0, 0.42],
        ),
      ),
      child: const SizedBox.expand(),
    );
  }
}

/// The fallback state's monogram tile: the entity's initials in a
/// serif face — a rounded square for groups, a circle for people (the
/// v11 distinction). Sits at the top of the identity column only when
/// there is no photo.
class ProfileMonogram extends StatelessWidget {
  const ProfileMonogram({
    super.key,
    required this.text,
    this.circular = false,
  });

  /// One-or-two-letter initials.
  final String text;

  /// People get circles; groups keep the squared tile.
  final bool circular;

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.primary(context);
    return Container(
      width: 58,
      height: 58,
      decoration: BoxDecoration(
        color: Color.alphaBlend(
          accent.withValues(alpha: 0.35),
          AppColors.darkBackground,
        ),
        borderRadius: BorderRadius.circular(circular ? 29 : 18),
      ),
      child: Center(
        child: Text(
          text,
          style: TextStyle(
            fontFamily: AppTheme.headingFont,
            fontSize: 24,
            fontWeight: FontWeight.w600,
            color: Color.lerp(accent, Colors.white, 0.55),
          ),
        ),
      ),
    );
  }
}
