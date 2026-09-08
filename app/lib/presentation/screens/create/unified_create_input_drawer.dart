import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/data/gen/ripls/api/unified_create_service.pb.dart'
    show DetectedContentType;
import 'package:ripls/presentation/viewmodels/unified_create_state.dart';
import 'package:ripls/presentation/viewmodels/unified_create_view_model.dart';
import 'package:ripls/presentation/widgets/accessibility/accessible_duration.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/create/unified_primary_button.dart';
import 'package:ripls/presentation/widgets/keyboard_dismiss_wrapper.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_surface.dart';

/// Bottom drawer for the unified-create input stage. Three tabs:
/// Text, Image, URL — laid out as a pill-shape segmented control.
class UnifiedCreateInputDrawer extends ConsumerStatefulWidget {
  const UnifiedCreateInputDrawer({super.key});

  @override
  ConsumerState<UnifiedCreateInputDrawer> createState() =>
      _UnifiedCreateInputDrawerState();
}

class _UnifiedCreateInputDrawerState
    extends ConsumerState<UnifiedCreateInputDrawer> {
  // Owned at drawer scope so `KeyboardActions` can register both nodes
  // up-front — the iOS "Done" toolbar wires off the focus-node list at
  // configure time, not per-panel. The text node drives the 4-5 line
  // textarea (multiline ignores `TextInputAction.done`); the URL node
  // drives the single-line field but still gets registered so the
  // toolbar shows for third-party keyboards that suppress the system
  // Done key.
  final FocusNode _textFocusNode = FocusNode(debugLabel: 'unifiedCreateText');
  final FocusNode _urlFocusNode = FocusNode(debugLabel: 'unifiedCreateUrl');

  @override
  void dispose() {
    _textFocusNode.dispose();
    _urlFocusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(unifiedCreateViewModelProvider);
    final vm = ref.read(unifiedCreateViewModelProvider.notifier);

    // No `KeyboardActions` wrapper — the drawer follows the keyboard up
    // via the outer `AnimatedPadding` in `UnifiedCreateModal`, and
    // `KeyboardDismissWrapper` plus the modal's tap-off Tappable handle
    // dismissal. The iOS-only "Done" toolbar the helper would install
    // is a redundant banner floating above the keyboard and breaks the
    // glass aesthetic (same change as the community-creation modal).
    return KeyboardDismissWrapper(
      child: GlassSurface(
          borderRadius: const BorderRadius.vertical(top: Radius.circular(28)),
          padding: const EdgeInsets.fromLTRB(16, 12, 16, 24),
          child: AnimatedSize(
            duration:
                accessibleDuration(context, const Duration(milliseconds: 200)),
            curve: Curves.easeOut,
            alignment: Alignment.topCenter,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const _DragHandle(),
                const SizedBox(height: 10),
                UnifiedPillTabs<CreateInputMode>(
                  value: state.inputMode,
                  entries: [
                    UnifiedPillTabEntry(
                        value: CreateInputMode.text,
                        label: context.l10n.unifiedCreateTabText),
                    UnifiedPillTabEntry(
                        value: CreateInputMode.image,
                        label: context.l10n.unifiedCreateTabImage),
                    UnifiedPillTabEntry(
                        value: CreateInputMode.url,
                        label: context.l10n.unifiedCreateTabUrl),
                  ],
                  onChange: vm.setInputMode,
                ),
                const SizedBox(height: 12),
                switch (state.inputMode) {
                  CreateInputMode.text => _TextPanel(
                      state: state,
                      vm: vm,
                      focusNode: _textFocusNode,
                    ),
                  // Image mode renders no body content in the drawer —
                  // the live camera preview + coaching carousel + shutter
                  // live in `UnifiedCreateCameraLayer` behind the drawer.
                  // The drawer collapses to the tab strip only.
                  CreateInputMode.image => const SizedBox.shrink(),
                  CreateInputMode.url => _UrlPanel(
                      state: state,
                      vm: vm,
                      focusNode: _urlFocusNode,
                    ),
                },
              ],
            ),
          ),
        ),
      );
  }
}

class _DragHandle extends StatelessWidget {
  const _DragHandle();
  @override
  Widget build(BuildContext context) {
    return Container(
      width: 40,
      height: 4,
      decoration: BoxDecoration(
        color: AppColors.modalDragHandle,
        borderRadius: BorderRadius.circular(2),
      ),
    );
  }
}

class _TextPanel extends StatefulWidget {
  const _TextPanel({
    required this.state,
    required this.vm,
    required this.focusNode,
  });
  final UnifiedCreateState state;
  final UnifiedCreateViewModel vm;
  final FocusNode focusNode;

  @override
  State<_TextPanel> createState() => _TextPanelState();
}

class _TextPanelState extends State<_TextPanel> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.state.prompt);

  // Cycling-hint state. Replaces the previous static suggested-prompts
  // UI (chips + example list): rather than show a separate selector,
  // we rotate through example prompts as the textarea's hint while the
  // field is empty. The user can read each example before it cycles to
  // the next. As soon as they start typing the hint is hidden (Flutter's
  // TextField behavior).
  //
  // Only for the undeclared blank create. When the caller seeded a
  // target type the user has already said what they are making, so the
  // tour is replaced by one static type-specific hint and the timer
  // never starts — a label that rewrites itself every three seconds is
  // also re-announced by screen readers (#2936).
  late List<String> _cyclingPrompts;
  int _cycleIndex = 0;
  Timer? _cycleTimer;
  static const _cycleInterval = Duration(seconds: 3);

  @override
  void initState() {
    super.initState();
    // Mixed across types so the user sees a Request, then an Event,
    // then an Item — gives a tour of what the tool can produce. Shuffled
    // on each open so repeat users don't always see the same first hint.
    _cyclingPrompts = [
      'Anyone have a hammer drill I could borrow this weekend?',
      'Going for a hike tomorrow morning at Mt Sanitas',
      'DeWalt 20V hammer drill with 2 batteries',
      'Looking for a hiking buddy tomorrow morning',
      'Hosting a Saturday potluck at my place, 6pm',
      'Climbing rope, 9.6mm, used a few times',
      'Need help moving a couch on Sunday',
      'Board game night Friday at the community center',
      'REI Half Dome 2-person tent, used a few seasons',
      'Anyone have a good sourdough starter to share?',
      'Sunday morning yoga in the park, all levels welcome',
      'Cast iron Dutch oven, Lodge brand, 6 quart',
    ]..shuffle();
    if (widget.state.targetType == null) {
      _cycleTimer = Timer.periodic(_cycleInterval, (_) {
        if (!mounted) return;
        setState(() {
          _cycleIndex = (_cycleIndex + 1) % _cyclingPrompts.length;
        });
      });
    }
  }

  /// The hint for a caller-declared type, or null for the blank create
  /// (which shows the rotating tour instead).
  String? _seededHint(BuildContext context) =>
      switch (widget.state.targetType) {
        DetectedContentType.DETECTED_CONTENT_TYPE_EVENT =>
          context.l10n.unifiedCreatePromptHintEvent,
        DetectedContentType.DETECTED_CONTENT_TYPE_REQUEST =>
          context.l10n.unifiedCreatePromptHintRequest,
        DetectedContentType.DETECTED_CONTENT_TYPE_GEAR =>
          context.l10n.unifiedCreatePromptHintItem,
        _ => null,
      };

  @override
  void didUpdateWidget(covariant _TextPanel old) {
    super.didUpdateWidget(old);
    // Sync controller when state.prompt changes from outside.
    // Skip when the controller already matches to avoid cursor jumps
    // while the user types.
    if (widget.state.prompt != _controller.text) {
      _controller.value = TextEditingValue(
        text: widget.state.prompt,
        selection: TextSelection.collapsed(offset: widget.state.prompt.length),
      );
    }
  }

  @override
  void dispose() {
    _cycleTimer?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final canGenerate = widget.state.prompt.trim().isNotEmpty;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _BorderlessGlassField(
          controller: _controller,
          focusNode: widget.focusNode,
          minLines: 4,
          maxLines: 5,
          hint: _seededHint(context) ?? _cyclingPrompts[_cycleIndex],
          textCapitalization: TextCapitalization.sentences,
          onChanged: widget.vm.setPrompt,
        ),
        const SizedBox(height: 12),
        UnifiedPrimaryButton(
          label: context.l10n.unifiedCreateGenerate,
          enabled: canGenerate,
          onTap: widget.vm.start,
        ),
      ],
    );
  }
}

class _UrlPanel extends StatefulWidget {
  const _UrlPanel({
    required this.state,
    required this.vm,
    required this.focusNode,
  });
  final UnifiedCreateState state;
  final UnifiedCreateViewModel vm;
  final FocusNode focusNode;

  @override
  State<_UrlPanel> createState() => _UrlPanelState();
}

class _UrlPanelState extends State<_UrlPanel> {
  late final TextEditingController _controller =
      TextEditingController(text: widget.state.urlInput);

  // Cycling-hint state. Mirrors _TextPanelState: rotate through
  // recognisable product + event site URLs as the field's hint so the
  // user knows what kinds of links the flow accepts. Hint hides as
  // soon as they paste / type.
  late List<String> _cyclingUrls;
  int _cycleIndex = 0;
  Timer? _cycleTimer;
  static const _cycleInterval = Duration(seconds: 3);

  @override
  void initState() {
    super.initState();
    // 5 product sites + 5 event sites — domain-level URLs so the hint
    // stays short and the user mentally maps "Oh, you can paste a REI
    // link / an Eventbrite link". Shuffled per open.
    _cyclingUrls = [
      // Product sites.
      'https://www.rei.com/...',
      'https://www.amazon.com/...',
      'https://www.patagonia.com/...',
      'https://www.dewalt.com/...',
      'https://www.backcountry.com/...',
      // Event sites.
      'https://www.eventbrite.com/...',
      'https://www.meetup.com/...',
      'https://allevents.in/...',
      'https://www.facebook.com/events/...',
      'https://partiful.com/...',
    ]..shuffle();
    _cycleTimer = Timer.periodic(_cycleInterval, (_) {
      if (!mounted) return;
      setState(() {
        _cycleIndex = (_cycleIndex + 1) % _cyclingUrls.length;
      });
    });
  }

  @override
  void didUpdateWidget(covariant _UrlPanel old) {
    super.didUpdateWidget(old);
    if (widget.state.urlInput != _controller.text) {
      _controller.value = TextEditingValue(
        text: widget.state.urlInput,
        selection:
            TextSelection.collapsed(offset: widget.state.urlInput.length),
      );
    }
  }

  @override
  void dispose() {
    _cycleTimer?.cancel();
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final canGenerate = widget.state.urlInput.trim().isNotEmpty;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        _BorderlessGlassField(
          controller: _controller,
          focusNode: widget.focusNode,
          minLines: 1,
          maxLines: 1,
          hint: _cyclingUrls[_cycleIndex],
          prefixIcon: Icons.link,
          textInputAction: TextInputAction.done,
          // URLs are case-sensitive — never autocapitalize.
          textCapitalization: TextCapitalization.none,
          onChanged: widget.vm.setUrlInput,
        ),
        const SizedBox(height: 12),
        UnifiedPrimaryButton(
          label: context.l10n.unifiedCreateGenerate,
          enabled: canGenerate,
          onTap: widget.vm.start,
        ),
      ],
    );
  }
}

/// Borderless translucent text field. Uses `modalInlineActionBackground`
/// only — no visible border, no Material outline, no focused-state
/// border ring.
class _BorderlessGlassField extends StatelessWidget {
  const _BorderlessGlassField({
    required this.hint,
    required this.onChanged,
    this.controller,
    this.focusNode,
    this.minLines = 1,
    this.maxLines = 1,
    this.prefixIcon,
    this.textInputAction,
    this.textCapitalization = TextCapitalization.none,
  });
  final String hint;
  final ValueChanged<String> onChanged;
  final TextEditingController? controller;
  final FocusNode? focusNode;
  final int minLines;
  final int maxLines;
  final IconData? prefixIcon;
  final TextInputAction? textInputAction;
  final TextCapitalization textCapitalization;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        // No fill — the drawer's glass surface shows through. Only a
        // hairline border gives the field its visual boundary.
        borderRadius: BorderRadius.circular(20),
        border: Border.all(color: AppColors.modalSearchFieldBorder),
      ),
      child: Row(
        // Single-line fields (URL) center-align the prefix icon with the
        // input text; multi-line fields (text-prompt textarea) top-align
        // so the icon sits at the first line.
        crossAxisAlignment:
            maxLines == 1 ? CrossAxisAlignment.center : CrossAxisAlignment.start,
        children: [
          if (prefixIcon != null) ...[
            Icon(prefixIcon, size: 18, color: AppColors.modalTextMuted.withValues(alpha: 0.6)),
            const SizedBox(width: 8),
          ],
          Expanded(
            child: TextField(
              controller: controller,
              focusNode: focusNode,
              minLines: minLines,
              maxLines: maxLines,
              onChanged: onChanged,
              textInputAction: textInputAction,
              textCapitalization: textCapitalization,
              cursorColor: AppColors.modalTextPrimary,
              style: TextStyle(color: AppColors.modalTextPrimary, fontSize: 14),
              decoration: InputDecoration(
                isCollapsed: true,
                filled: false,
                fillColor: Colors.transparent,
                border: InputBorder.none,
                enabledBorder: InputBorder.none,
                focusedBorder: InputBorder.none,
                hintText: hint,
                hintStyle: TextStyle(color: AppColors.modalTextMuted.withValues(alpha: 0.55)),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Pill-shape segmented control. Single-select. The selected entry gets
/// the [AppColors.modalChipBackgroundActive] (white) fill; unselected
/// entries are transparent. No border, no checkmark.
class UnifiedPillTabs<T> extends StatelessWidget {
  const UnifiedPillTabs({
    super.key,
    required this.value,
    required this.entries,
    required this.onChange,
  });

  final T value;
  final List<UnifiedPillTabEntry<T>> entries;
  final ValueChanged<T> onChange;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(4),
      decoration: BoxDecoration(
        color: AppColors.modalChipBackground,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Row(
        children: [
          for (final entry in entries)
            Expanded(
              child: _PillTabButton(
                label: entry.label,
                selected: value == entry.value,
                onTap: () => onChange(entry.value),
              ),
            ),
        ],
      ),
    );
  }
}

class UnifiedPillTabEntry<T> {
  const UnifiedPillTabEntry({required this.value, required this.label});
  final T value;
  final String label;
}

class _PillTabButton extends StatelessWidget {
  const _PillTabButton({
    required this.label,
    required this.selected,
    required this.onTap,
  });
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      inkBorderRadius: BorderRadius.circular(999),
      child: AnimatedContainer(
        duration: accessibleDuration(context, const Duration(milliseconds: 150)),
        padding: const EdgeInsets.symmetric(vertical: 8),
        decoration: BoxDecoration(
          color: selected ? AppColors.modalChipBackgroundActive : Colors.transparent,
          borderRadius: BorderRadius.circular(999),
        ),
        child: Center(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: selected ? AppColors.modalChipTextActive : AppColors.modalChipText,
            ),
          ),
        ),
      ),
    );
  }
}
