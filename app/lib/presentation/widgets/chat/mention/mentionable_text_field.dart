import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import 'mention_suggestion_item.dart';
import 'mention_suggestion_overlay.dart';
import 'mention_text_controller.dart';
import 'mention_types.dart';

/// A TextField that supports @-mention autocomplete.
///
/// Shows a suggestion overlay when the user types @ and filters results
/// as they continue typing. Supports keyboard navigation and selection.
class MentionableTextField extends StatefulWidget {
  /// The controller for the text field.
  final MentionTextController controller;

  /// The focus node for the text field.
  final FocusNode focusNode;

  /// Callback to fetch suggestions for a query.
  ///
  /// Called when the user types @ followed by characters. Should return
  /// a list of suggestions matching the query.
  final Future<List<MentionSuggestion>> Function(String query) onFetchSuggestions;

  /// Optional callback to load media URLs for thumbnails.
  ///
  /// Called for each suggestion that has a mediaId to load its thumbnail.
  final MediaUrlLoader? mediaUrlLoader;

  /// Hint text to display when the field is empty.
  final String? hintText;

  /// Text style for the input.
  final TextStyle? style;

  /// Input decoration for the field.
  final InputDecoration? decoration;

  /// Maximum number of lines.
  final int? maxLines;

  /// Text capitalization.
  final TextCapitalization textCapitalization;

  /// Text input action.
  final TextInputAction? textInputAction;

  /// Callback when the user submits (presses enter on keyboard).
  final ValueChanged<String>? onSubmitted;

  /// Debounce duration for fetching suggestions.
  final Duration debounceDuration;

  /// When true, wraps the suggestion overlay in a dark theme so that
  /// theme-aware colors (AppColors) render with dark-mode values.  Use this
  /// when the text field sits on the dark immersive content-view background.
  final bool immersive;

  const MentionableTextField({
    super.key,
    required this.controller,
    required this.focusNode,
    required this.onFetchSuggestions,
    this.mediaUrlLoader,
    this.hintText,
    this.style,
    this.decoration,
    this.maxLines,
    this.textCapitalization = TextCapitalization.sentences,
    this.textInputAction,
    this.onSubmitted,
    this.debounceDuration = const Duration(milliseconds: 200),
    this.immersive = false,
  });

  @override
  State<MentionableTextField> createState() => _MentionableTextFieldState();
}

class _MentionableTextFieldState extends State<MentionableTextField> {
  final LayerLink _layerLink = LayerLink();
  OverlayEntry? _overlayEntry;

  List<MentionSuggestion> _suggestions = [];
  int _highlightedIndex = -1;
  bool _isLoading = false;
  String _currentQuery = '';
  String? _errorMessage;
  Timer? _debounceTimer;

  @override
  void initState() {
    super.initState();
    widget.controller.onMentionQueryChanged = _onMentionQueryChanged;
    widget.focusNode.addListener(_onFocusChanged);
  }

  @override
  void dispose() {
    _debounceTimer?.cancel();
    _removeOverlay();
    widget.focusNode.removeListener(_onFocusChanged);
    super.dispose();
  }

  @override
  void didUpdateWidget(MentionableTextField oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.controller != widget.controller) {
      oldWidget.controller.onMentionQueryChanged = null;
      widget.controller.onMentionQueryChanged = _onMentionQueryChanged;
    }
    if (oldWidget.focusNode != widget.focusNode) {
      oldWidget.focusNode.removeListener(_onFocusChanged);
      widget.focusNode.addListener(_onFocusChanged);
    }
  }

  void _onFocusChanged() {
    if (!widget.focusNode.hasFocus) {
      _removeOverlay();
    }
  }

  void _onMentionQueryChanged(MentionQuery? query) {
    if (query == null) {
      _removeOverlay();
      return;
    }

    _currentQuery = query.queryText;
    _showOverlay();
    _fetchSuggestionsDebounced(query.queryText);
  }

  void _fetchSuggestionsDebounced(String query) {
    _debounceTimer?.cancel();

    // Show loading state immediately
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });
    _updateOverlay();

    _debounceTimer = Timer(widget.debounceDuration, () {
      _fetchSuggestions(query);
    });
  }

  Future<void> _fetchSuggestions(String query) async {
    try {
      final suggestions = await widget.onFetchSuggestions(query);

      // Check if we're still in mention mode and query hasn't changed
      if (!mounted || !widget.controller.isInMentionMode) return;
      if (widget.controller.currentQuery?.queryText != query) return;

      setState(() {
        _suggestions = suggestions;
        _highlightedIndex = suggestions.isNotEmpty ? 0 : -1;
        _isLoading = false;
        _errorMessage = null;
      });
      _updateOverlay();
    } catch (e) {
      if (!mounted) return;

      // Determine user-friendly error message
      String errorMessage = 'Unable to search';
      if (e.toString().contains('SocketException') ||
          e.toString().contains('network') ||
          e.toString().contains('Failed host lookup')) {
        errorMessage = 'No internet connection';
      }

      setState(() {
        _suggestions = [];
        _highlightedIndex = -1;
        _isLoading = false;
        _errorMessage = errorMessage;
      });
      _updateOverlay();
    }
  }

  void _retryFetch() {
    if (_currentQuery.isNotEmpty) {
      _fetchSuggestionsDebounced(_currentQuery);
    }
  }

  void _showOverlay() {
    if (_overlayEntry != null) return;

    _overlayEntry = OverlayEntry(
      builder: (context) => _buildOverlayContent(),
    );

    Overlay.of(context).insert(_overlayEntry!);
  }

  void _updateOverlay() {
    _overlayEntry?.markNeedsBuild();
  }

  void _removeOverlay() {
    _debounceTimer?.cancel();
    _overlayEntry?.remove();
    _overlayEntry = null;
    _suggestions = [];
    _highlightedIndex = -1;
    _isLoading = false;
    _currentQuery = '';
    _errorMessage = null;
  }

  Widget _buildOverlayContent() {
    // Calculate overlay width based on text field's actual width
    final renderBox = context.findRenderObject() as RenderBox?;
    final textFieldWidth = renderBox?.size.width ?? 300;

    Widget overlay = MentionSuggestionOverlay(
      suggestions: _suggestions,
      highlightedIndex: _highlightedIndex,
      isLoading: _isLoading,
      query: _currentQuery,
      mediaUrlLoader: widget.mediaUrlLoader,
      onSelect: _onSuggestionSelected,
      errorMessage: _errorMessage,
      onRetry: _retryFetch,
    );

    // When rendering on the dark immersive content-view background, wrap in a
    // dark Theme so that theme-aware AppColors automatically pick dark values.
    if (widget.immersive) {
      overlay = Theme(
        data: Theme.of(context).copyWith(brightness: Brightness.dark),
        child: overlay,
      );
    }

    return Positioned(
      width: textFieldWidth,
      child: CompositedTransformFollower(
        link: _layerLink,
        showWhenUnlinked: false,
        offset: const Offset(0, -8),
        followerAnchor: Alignment.bottomLeft,
        targetAnchor: Alignment.topLeft,
        child: Material(
          type: MaterialType.transparency,
          child: overlay,
        ),
      ),
    );
  }

  void _onSuggestionSelected(MentionSuggestion suggestion) {
    widget.controller.insertMention(suggestion);
    _removeOverlay();
  }

  KeyEventResult _handleKeyEvent(FocusNode node, KeyEvent event) {
    if (!widget.controller.isInMentionMode || _overlayEntry == null) {
      return KeyEventResult.ignored;
    }

    if (event is! KeyDownEvent && event is! KeyRepeatEvent) {
      return KeyEventResult.ignored;
    }

    final key = event.logicalKey;

    // Handle arrow keys for navigation
    if (key == LogicalKeyboardKey.arrowDown) {
      _navigateDown();
      return KeyEventResult.handled;
    }

    if (key == LogicalKeyboardKey.arrowUp) {
      _navigateUp();
      return KeyEventResult.handled;
    }

    // Handle enter to select
    if (key == LogicalKeyboardKey.enter) {
      if (_highlightedIndex >= 0 && _highlightedIndex < _suggestions.length) {
        _onSuggestionSelected(_suggestions[_highlightedIndex]);
        return KeyEventResult.handled;
      }
    }

    // Handle escape to dismiss
    if (key == LogicalKeyboardKey.escape) {
      widget.controller.cancelMention();
      _removeOverlay();
      return KeyEventResult.handled;
    }

    return KeyEventResult.ignored;
  }

  void _navigateDown() {
    if (_suggestions.isEmpty) return;

    setState(() {
      _highlightedIndex = (_highlightedIndex + 1) % _suggestions.length;
    });
    _updateOverlay();
  }

  void _navigateUp() {
    if (_suggestions.isEmpty) return;

    setState(() {
      if (_highlightedIndex <= 0) {
        _highlightedIndex = _suggestions.length - 1;
      } else {
        _highlightedIndex--;
      }
    });
    _updateOverlay();
  }

  @override
  Widget build(BuildContext context) {
    return CompositedTransformTarget(
      link: _layerLink,
      child: Focus(
        onKeyEvent: _handleKeyEvent,
        child: TextField(
          controller: widget.controller,
          focusNode: widget.focusNode,
          style: widget.style,
          decoration: widget.decoration ??
              InputDecoration(
                hintText: widget.hintText,
                contentPadding: const EdgeInsets.symmetric(
                  horizontal: 12,
                  vertical: 12,
                ),
              ),
          maxLines: widget.maxLines,
          textCapitalization: widget.textCapitalization,
          textInputAction: widget.textInputAction,
          onSubmitted: widget.onSubmitted,
        ),
      ),
    );
  }
}
