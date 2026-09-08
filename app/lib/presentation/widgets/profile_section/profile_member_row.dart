import 'package:flutter/material.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/profile_section/profile_face_stack.dart';

/// The relationship/members row (issue #2568,
/// `profile-final-hybrid-v2.html` State A): a bigger face stack beside
/// a bold title line ("You & Betty" / "Thomas, Alfred & Betty") and an
/// optional muted subtitle (shared interests, member count). Sits
/// below the hero, above the stat rows, on the active-ask state only.
///
/// Renders over the profile's full-bleed background photo, so all text
/// is light. Wrapped in [Tappable] only when [onTap] is provided —
/// the person variant has nowhere to route to and renders as plain,
/// non-interactive text.
class ProfileMemberRow extends StatelessWidget {
  const ProfileMemberRow({
    super.key,
    required this.faces,
    required this.title,
    this.subtitle,
    this.onTap,
    this.onTapRect,
    this.semanticsLabel,
  });

  final List<FaceStackEntry> faces;
  final String title;
  final String? subtitle;

  /// Group variant opens the members list; person variant passes null.
  final VoidCallback? onTap;

  /// When set, preferred over [onTap]: receives the row's own on-screen
  /// footprint — the source rect a morph-reveal panel grows from (see
  /// `openContentMorphPanel`), so the members list expands in place the
  /// same way the profile conversation does.
  final ValueChanged<Rect>? onTapRect;

  /// Required when the row is tappable (announced by the [Tappable]).
  final String? semanticsLabel;

  @override
  Widget build(BuildContext context) {
    final interactive = onTap != null || onTapRect != null;
    final row = Padding(
      padding: const EdgeInsets.fromLTRB(22, 6, 22, 6),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          ProfileFaceStack(faces: faces, size: 30),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: const TextStyle(
                    fontSize: 13.5,
                    fontWeight: FontWeight.w600,
                    color: Colors.white,
                  ),
                ),
                if (subtitle != null && subtitle!.isNotEmpty)
                  Text(
                    subtitle!,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 11.5,
                      letterSpacing: 0.02,
                      color: Colors.white.withValues(alpha: 0.70),
                    ),
                  ),
              ],
            ),
          ),
          // The v11 chevron — only when the row actually goes somewhere
          // (the group's member list); the person variant stays plain.
          if (interactive)
            Icon(
              Icons.chevron_right,
              size: 15,
              color: Colors.white.withValues(alpha: 0.52),
            ),
        ],
      ),
    );
    if (!interactive) return row;
    return Builder(
      builder: (ctx) => Tappable(
        semanticsLabel: semanticsLabel ?? title,
        // Rect-aware rows grow a morph-reveal members panel from their own
        // footprint (the in-place expansion the profile conversation uses).
        onTap: onTapRect != null ? () => onTapRect!(_boundsOf(ctx)) : onTap,
        child: row,
      ),
    );
  }
}

/// The tapped row's own on-screen footprint — the source rect a
/// morph-reveal panel grows from (`openContentMorphPanel`). Mirrors the
/// content views' per-file `_rectOf` helpers; [context] must belong to
/// an already-laid-out element (safe inside a tap handler).
Rect _boundsOf(BuildContext context) {
  final box = context.findRenderObject();
  return box is RenderBox && box.hasSize
      ? box.localToGlobal(Offset.zero) & box.size
      : Rect.zero;
}
