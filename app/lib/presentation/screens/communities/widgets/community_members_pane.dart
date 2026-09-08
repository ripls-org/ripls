import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/community_service.pb.dart' show CommunityMember;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/community_content_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/services/providers.dart';

/// CommunityMembersPane shows a scrollable list of community members.
class CommunityMembersPane extends ConsumerWidget {
  final String communityId;

  const CommunityMembersPane({super.key, required this.communityId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(communityContentProvider);

    return SingleChildScrollView(
      padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          if (state.isLoadingMembers)
            const Center(child: CircularProgressIndicator())
          else if (state.members.isEmpty)
            _EmptyMembers()
          else
            _MemberList(members: state.members),
        ],
      ),
    );
  }
}

class _MemberList extends StatelessWidget {
  final List<CommunityMember> members;

  const _MemberList({required this.members});

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        for (final member in members)
          _MemberRow(member: member),
      ],
    );
  }
}

class _MemberRow extends ConsumerWidget {
  final CommunityMember member;

  const _MemberRow({required this.member});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final user = member.hasUser() ? member.user : User();
    final mediaId = user.mediaId;

    String? avatarUrl;
    if (mediaId.isNotEmpty) {
      avatarUrl = ref.watch(mediaUrlProvider(mediaId)).asData?.value;
    }

    return Tappable(
      semanticsLabel: user.name,
      onTap: user.id.isNotEmpty
          ? () => ContentViewHelpers.openUserScreen(context, user.id)
          : null,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Row(
          children: [
            _Avatar(mediaId: mediaId, imageUrl: avatarUrl, name: user.name),
            const SizedBox(width: 12),
            Expanded(
              child: Text(
                user.name.isEmpty ? '—' : user.name,
                style: const TextStyle(
                  color: Colors.white,
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ),
            if (user.id.isNotEmpty)
              Icon(
                Icons.chevron_right,
                color: Colors.white.withValues(alpha: 0.3),
                size: 18,
              ),
          ],
        ),
      ),
    );
  }
}

class _Avatar extends StatelessWidget {
  final String mediaId;
  final String? imageUrl;
  final String name;

  const _Avatar({
    required this.mediaId,
    required this.imageUrl,
    required this.name,
  });

  @override
  Widget build(BuildContext context) {
    final url = imageUrl ?? '';
    if (url.isNotEmpty) {
      return ClipOval(
        child: CachedMediaImage(
          imageUrl: url,
          cacheKey: mediaId.isNotEmpty ? mediaId : null,
          width: 36,
          height: 36,
          fit: BoxFit.cover,
          semanticsLabel: name,
        ),
      );
    }
    return CircleAvatar(
      radius: 18,
      backgroundColor: AppColors.primary(context).withValues(alpha: 0.3),
      child: Text(
        name.isNotEmpty ? name[0].toUpperCase() : '?',
        style: const TextStyle(
          color: Colors.white,
          fontWeight: FontWeight.w700,
          fontSize: 14,
        ),
      ),
    );
  }
}

class _EmptyMembers extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.only(top: 24),
        child: Text(
          context.l10n.communityMembersEmpty,
          style: TextStyle(
            color: Colors.white.withValues(alpha: 0.4),
            fontSize: 14,
          ),
        ),
      ),
    );
  }
}
