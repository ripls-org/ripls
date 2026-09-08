import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/data/gen/ripls/api/feed_service.pb.dart' show StoryPayload, EmbeddedEsmPrompt;
import 'package:ripls/data/gen/ripls/api/user.pb.dart' show User;
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/accessibility/toggle.dart';
import 'package:ripls/presentation/widgets/content/content_view_helpers.dart';
import 'package:ripls/presentation/widgets/user_avatar.dart';
import 'package:ripls/services/providers/esm_providers.dart';

/// Stable option keys the v1 server emits on the embedded prompt.
const String _kOptionDoAgain = 'do_again';
const String _kOptionMaybe = 'maybe';
const String _kOptionNotForMe = 'not_for_me';

final _log = Logger('StoryEmbeddedEsm');

/// StoryEmbeddedEsm renders the bottom voting block on a Story when the
/// server populates [StoryPayload.embeddedEsmPrompt]. Flips between a
/// voting state (three options) and a voted state (social-proof avatars
/// for Yes; soft acknowledgement for Maybe / Not for me) with a "change"
/// affordance that returns to the voting state.
///
/// Submits votes through [esmRepositoryProvider]; optimistic updates keep
/// the UI snappy and roll back on error.
class StoryEmbeddedEsm extends ConsumerStatefulWidget {
  final StoryPayload story;

  const StoryEmbeddedEsm({super.key, required this.story});

  @override
  ConsumerState<StoryEmbeddedEsm> createState() => _StoryEmbeddedEsmState();
}

class _StoryEmbeddedEsmState extends ConsumerState<StoryEmbeddedEsm> {
  String? _localVote;
  List<User>? _localSocialProofUsers;
  int? _localSocialProofRemainder;
  bool _changing = false;
  bool _submitting = false;

  EmbeddedEsmPrompt get _prompt => widget.story.embeddedEsmPrompt;

  String? get _currentVote {
    if (_changing) return null;
    if (_localVote != null) return _localVote;
    if (!_prompt.hasCurrentResponseOptionKey()) return null;
    final key = _prompt.currentResponseOptionKey;
    return key.isEmpty ? null : key;
  }

  List<User> get _socialProofUsers =>
      _localSocialProofUsers ?? widget.story.embeddedEsmSocialProofUsers;

  int get _socialProofRemainder =>
      _localSocialProofRemainder ?? widget.story.embeddedEsmSocialProofRemainder;

  @override
  Widget build(BuildContext context) {
    final vote = _currentVote;
    if (vote == null || vote.isEmpty) {
      return _buildVoting(context);
    }
    return _buildVoted(context, vote);
  }

  Widget _buildVoting(BuildContext context) {
    final question = _prompt.question.isNotEmpty
        ? _prompt.question
        : context.l10n.esmStoryQuestionDoAgain;

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          question,
          textAlign: TextAlign.center,
          style: const TextStyle(
            fontFamily: AppTheme.headingFont,
            color: Colors.white,
            fontSize: 14.5,
            height: 1.4,
            shadows: [
              Shadow(
                offset: Offset(0, 1),
                blurRadius: 6,
                color: Colors.black45,
              ),
            ],
          ),
        ),
        const SizedBox(height: 14),
        Row(
          mainAxisAlignment: MainAxisAlignment.center,
          mainAxisSize: MainAxisSize.min,
          children: _buildVotingOptions(context),
        ),
      ],
    );
  }

  List<Widget> _buildVotingOptions(BuildContext context) {
    final options = _prompt.responseOptions.isNotEmpty
        ? _prompt.responseOptions.map((o) => (key: o.key, label: o.label)).toList()
        : <({String key, String label})>[
            (key: _kOptionDoAgain, label: context.l10n.esmStoryOptionYes),
            (key: _kOptionMaybe, label: context.l10n.esmStoryOptionMaybe),
            (key: _kOptionNotForMe, label: context.l10n.esmStoryOptionNo),
          ];

    final widgets = <Widget>[];
    for (var i = 0; i < options.length; i++) {
      final opt = options[i];
      if (i > 0) widgets.add(const SizedBox(width: 8));
      widgets.add(_buildVoteButton(context, opt.key, opt.label));
    }
    return widgets;
  }

  Widget _buildVoteButton(BuildContext context, String optionKey, String label) {
    final primary = optionKey == _kOptionDoAgain;
    final semantics = switch (optionKey) {
      _kOptionDoAgain => context.l10n.a11yEsmVoteYes,
      _kOptionMaybe => context.l10n.a11yEsmVoteMaybe,
      _kOptionNotForMe => context.l10n.a11yEsmVoteNo,
      _ => label,
    };
    return Toggle(
      semanticsLabel: semantics,
      selected: _currentVote == optionKey,
      onTap: _submitting ? null : () => _onVote(optionKey),
      child: Container(
        constraints: const BoxConstraints(minHeight: 48, minWidth: 48),
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 9),
        decoration: BoxDecoration(
          borderRadius: BorderRadius.circular(999),
          color: primary
              ? AppColors.transferCoral
              : Colors.white.withValues(alpha: 0.14),
          border: primary
              ? null
              : Border.all(color: Colors.white.withValues(alpha: 0.28)),
        ),
        child: Center(
          child: Text(
            label,
            style: TextStyle(
              color: Colors.white,
              fontSize: 13,
              fontWeight: primary ? FontWeight.w700 : FontWeight.w600,
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildVoted(BuildContext context, String vote) {
    final isYes = vote == _kOptionDoAgain;
    final isMaybe = vote == _kOptionMaybe;
    final isNo = vote == _kOptionNotForMe;

    final headerLabel = isYes
        ? context.l10n.esmStoryVotedYesGoodCompany
        : isMaybe
            ? context.l10n.esmStoryVotedMaybe
            : isNo
                ? context.l10n.esmStoryVotedNo
                : null;

    return Semantics(
      liveRegion: true,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (headerLabel != null) ...[
            Text(
              headerLabel,
              textAlign: TextAlign.center,
              style: TextStyle(
                color: Colors.white.withValues(alpha: 0.78),
                fontSize: 11,
                fontWeight: FontWeight.w600,
                letterSpacing: 0.3,
              ),
            ),
            const SizedBox(height: 10),
          ],
          if (isYes) _buildSocialProofRow(context),
          if (isMaybe || isNo) _buildSelfOnlyRow(context),
          const SizedBox(height: 8),
          if (isYes && _socialProofUsers.isNotEmpty)
            _buildSocialProofDescriptor(context),
          if (isMaybe) _descriptorLine(context.l10n.esmStoryVotedMaybeDescriptor),
          if (isNo) _descriptorLine(context.l10n.esmStoryVotedNoDescriptor),
          const SizedBox(height: 4),
          Tappable(
            semanticsLabel: context.l10n.a11yEsmChangeVote,
            onTap: _submitting ? null : _onChange,
            isLink: true,
            child: Padding(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
              child: Text(
                context.l10n.esmStoryChange,
                style: TextStyle(
                  color: Colors.white.withValues(alpha: 0.6),
                  fontSize: 11,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }

  Widget _descriptorLine(String text) {
    return Text(
      text,
      textAlign: TextAlign.center,
      style: TextStyle(
        color: Colors.white.withValues(alpha: 0.9),
        fontSize: 13,
        height: 1.4,
        shadows: const [
          Shadow(
            offset: Offset(0, 1),
            blurRadius: 6,
            color: Colors.black45,
          ),
        ],
      ),
    );
  }

  Widget _buildSelfOnlyRow(BuildContext context) {
    return const Center(
      child: SizedBox(width: 38, height: 38, child: _SelfAvatarPlaceholder()),
    );
  }

  Widget _buildSocialProofRow(BuildContext context) {
    final users = _socialProofUsers;
    final remainder = _socialProofRemainder;
    final children = <Widget>[];

    children.add(_avatarChip(context, null, isSelf: true));
    for (final u in users) {
      children.add(const SizedBox(width: 4));
      children.add(_avatarChip(context, u));
    }
    if (remainder > 0) {
      children.add(const SizedBox(width: 4));
      children.add(_remainderChip(context, remainder));
    }

    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      mainAxisSize: MainAxisSize.min,
      children: children,
    );
  }

  Widget _avatarChip(BuildContext context, User? user, {bool isSelf = false}) {
    const radius = 17.0;
    const size = radius * 2;

    if (isSelf) {
      return const SizedBox(
        width: size,
        height: size,
        child: _SelfAvatarPlaceholder(),
      );
    }
    return SizedBox(
      width: size,
      height: size,
      child: Semantics(
        label: context.l10n.a11yEsmPhotoOfUser(user!.name),
        image: true,
        child: Tappable(
          semanticsLabel: context.l10n.a11yEsmPhotoOfUser(user.name),
          onTap: () => ContentViewHelpers.openUserScreen(context, user.id),
          child: UserAvatar(user: user, radius: radius - 2),
        ),
      ),
    );
  }

  Widget _remainderChip(BuildContext context, int remainder) {
    return Container(
      width: 34,
      height: 34,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        color: Colors.white.withValues(alpha: 0.18),
        borderRadius: BorderRadius.circular(17),
        border: Border.all(color: Colors.white.withValues(alpha: 0.85), width: 2),
      ),
      child: Text(
        '+$remainder',
        style: const TextStyle(
          color: Colors.white,
          fontSize: 11,
          fontWeight: FontWeight.w700,
        ),
      ),
    );
  }

  Widget _buildSocialProofDescriptor(BuildContext context) {
    final users = _socialProofUsers;
    final remainder = _socialProofRemainder;
    final names = users.map((u) => _firstName(u.name)).toList();

    String text;
    final l10n = context.l10n;
    if (names.isEmpty) {
      text = '';
    } else if (names.length == 1 && remainder == 0) {
      text = l10n.esmStorySocialProofOneName(names[0]);
    } else if (names.length == 2 && remainder == 0) {
      text = l10n.esmStorySocialProofTwoNames(names[0], names[1]);
    } else if (names.length == 3 && remainder == 0) {
      text = l10n.esmStorySocialProofThreeNames(names[0], names[1], names[2]);
    } else if (names.length >= 3 && remainder > 0) {
      text = l10n.esmStorySocialProofThreePlusOthers(
          names[0], names[1], names[2], remainder);
    } else {
      text = l10n.esmStorySocialProofOneName(names.first);
    }
    if (text.isEmpty) return const SizedBox.shrink();
    return _descriptorLine(text);
  }

  Future<void> _onVote(String optionKey) async {
    final repo = ref.read(esmRepositoryProvider);
    final previousVote = _localVote;
    final previousUsers = _localSocialProofUsers;
    final previousRemainder = _localSocialProofRemainder;

    setState(() {
      _localVote = optionKey;
      _changing = false;
      _submitting = true;
    });

    try {
      await repo.respond(
        promptId: _prompt.promptId,
        responseOptionKey: optionKey,
      );
    } catch (e, st) {
      _log.warning('ESM vote submission failed', e, st);
      if (!mounted) return;
      setState(() {
        _localVote = previousVote;
        _localSocialProofUsers = previousUsers;
        _localSocialProofRemainder = previousRemainder;
      });
    } finally {
      if (mounted) {
        setState(() => _submitting = false);
      }
    }
  }

  void _onChange() {
    setState(() => _changing = true);
  }

  String _firstName(String full) {
    final t = full.trim();
    if (t.isEmpty) return t;
    final i = t.indexOf(' ');
    return i < 0 ? t : t.substring(0, i);
  }
}

class _SelfAvatarPlaceholder extends StatelessWidget {
  const _SelfAvatarPlaceholder();

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: AppColors.lightGradient,
        ),
        border: Border.all(color: Colors.white.withValues(alpha: 0.85), width: 2),
      ),
      alignment: Alignment.center,
      child: const Icon(Icons.check, color: Colors.white, size: 16),
    );
  }
}
