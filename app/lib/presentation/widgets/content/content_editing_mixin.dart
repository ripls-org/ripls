import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Mixin for managing editing buffer state in content views and preview modals.
///
/// Provides standard pattern for title/description editing with initialization.
/// This eliminates the duplication of editing buffer management across content types.
///
/// Example usage:
/// ```dart
/// class _GearContentViewState extends ConsumerState<GearContentView>
///     with ContentEditingMixin {
///
///   @override
///   void didUpdateWidget(GearContentView oldWidget) {
///     super.didUpdateWidget(oldWidget);
///     final gear = ref.read(gearProvider(widget.gearId)).gearDetails;
///     if (gear != null) {
///       initializeEditing(
///         initialTitle: gear.name,
///         initialDescription: gear.description,
///       );
///     }
///   }
///
///   Widget _buildEditableTitle() {
///     return TextField(
///       controller: TextEditingController(text: editingTitle),
///       onChanged: updateEditingTitle,
///     );
///   }
/// }
/// ```
mixin ContentEditingMixin<T extends ConsumerStatefulWidget>
    on ConsumerState<T> {
  // Editing buffer
  String _editingTitle = '';
  String _editingDescription = '';
  bool _hasInitializedEditing = false;

  // Getters for widgets to access
  String get editingTitle => _editingTitle;
  String get editingDescription => _editingDescription;
  bool get hasInitializedEditing => _hasInitializedEditing;

  /// Initialize editing buffer from provided values (call in initState or didUpdateWidget).
  ///
  /// This method is idempotent - it only initializes once per widget lifecycle.
  /// To reinitialize, call [resetEditing] first.
  void initializeEditing({
    required String initialTitle,
    required String initialDescription,
  }) {
    if (!_hasInitializedEditing) {
      setState(() {
        _editingTitle = initialTitle;
        _editingDescription = initialDescription;
        _hasInitializedEditing = true;
      });
    }
  }

  /// Update title in editing buffer.
  void updateEditingTitle(String value) {
    setState(() {
      _editingTitle = value;
    });
  }

  /// Update description in editing buffer.
  void updateEditingDescription(String value) {
    setState(() {
      _editingDescription = value;
    });
  }

  /// Reset editing buffer (call when canceling edits).
  void resetEditing() {
    setState(() {
      _editingTitle = '';
      _editingDescription = '';
      _hasInitializedEditing = false;
    });
  }
}
