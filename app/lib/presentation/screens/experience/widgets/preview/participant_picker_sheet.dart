import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/viewmodels/gen_experience_view_model.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass.dart';
import 'package:ripls/services/providers.dart';

/// ParticipantPickerSheet is a bottom sheet for tagging community members as
/// participants in an experience.
///
/// Allows searching by name and toggling each member's participation.
/// AI-extracted [mentionedNames] are shown as chips to help seed the search
/// input.
class ParticipantPickerSheet extends ConsumerStatefulWidget {
  const ParticipantPickerSheet({
    super.key,
    required this.communityId,
    required this.taggedParticipantIds,
    required this.mentionedNames,
  });

  final String communityId;
  final List<String> taggedParticipantIds;
  final List<String> mentionedNames;

  @override
  ConsumerState<ParticipantPickerSheet> createState() =>
      _ParticipantPickerSheetState();
}

class _ParticipantPickerSheetState
    extends ConsumerState<ParticipantPickerSheet> {
  final _searchController = TextEditingController();
  List<User> _searchResults = [];
  bool _isSearching = false;

  @override
  void initState() {
    super.initState();
    // Load all members on open so the list is immediately populated.
    _search('');
  }

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  Future<void> _search(String query) async {
    setState(() => _isSearching = true);
    try {
      final repo = ref.read(communityRepositoryProvider);
      final results = await repo.searchMembers(
        communityId: widget.communityId,
        query: query,
        limit: 20,
      );
      if (mounted) setState(() => _searchResults = results);
    } catch (_) {
      // Non-fatal: leave previous results visible.
    } finally {
      if (mounted) setState(() => _isSearching = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tagged = ref.watch(genExperienceProvider).taggedParticipantIds;

    return SizedBox(
      height: MediaQuery.of(context).size.height * 0.65,
      child: GlassSheet(
        padding: EdgeInsets.zero,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _buildHeader(context),
            if (widget.mentionedNames.isNotEmpty)
              _buildMentionedNameChips(context),
            _buildSearchField(context),
            Expanded(child: _buildMemberList(context, tagged)),
          ],
        ),
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
      child: Text(
        'Tag participants',
        style: Theme.of(context).textTheme.titleMedium?.copyWith(
              fontWeight: FontWeight.w600,
              color: AppColors.modalTextPrimary,
            ),
      ),
    );
  }

  Widget _buildMentionedNameChips(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 10, 16, 0),
      child: Wrap(
        spacing: 8,
        runSpacing: 4,
        children: widget.mentionedNames.map((name) {
          return ActionChip(
            label: Text(name),
            avatar: const Icon(Icons.search, size: 16),
            onPressed: () {
              _searchController.text = name;
              _search(name);
            },
          );
        }).toList(),
      ),
    );
  }

  Widget _buildSearchField(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 12, 16, 4),
      child: GlassSearchInput(
        controller: _searchController,
        focusNode: FocusNode(),
        hintText: 'Search members…',
        onChanged: (v) => _search(v.trim()),
        onSubmitted: (v) => _search(v.trim()),
      ),
    );
  }

  Widget _buildMemberList(BuildContext context, List<String> tagged) {
    if (_isSearching && _searchResults.isEmpty) {
      return const Center(
        child: CircularProgressIndicator(
          valueColor: AlwaysStoppedAnimation(AppColors.modalTextPrimary),
        ),
      );
    }
    if (_searchResults.isEmpty) {
      return Center(
        child: Text(
          'No members found',
          style: TextStyle(color: AppColors.modalTextSecondary),
        ),
      );
    }
    return ListView.builder(
      itemCount: _searchResults.length,
      itemBuilder: (context, index) {
        final user = _searchResults[index];
        final isTagged = tagged.contains(user.id);
        return CheckboxListTile(
          value: isTagged,
          title: Text(
            user.name,
            style: const TextStyle(color: AppColors.modalTextPrimary),
          ),
          checkColor: AppColors.modalChipTextActive,
          activeColor: AppColors.modalPrimaryButtonBackground,
          secondary: CircleAvatar(
            backgroundColor: AppColors.modalInsetCardBg,
            child: Text(
              user.name.isNotEmpty ? user.name[0] : '?',
              style: const TextStyle(color: AppColors.modalTextPrimary),
            ),
          ),
          onChanged: (_) {
            if (isTagged) {
              ref
                  .read(genExperienceProvider.notifier)
                  .untagParticipant(user.id);
            } else {
              ref
                  .read(genExperienceProvider.notifier)
                  .tagParticipant(user.id);
            }
          },
        );
      },
    );
  }
}
