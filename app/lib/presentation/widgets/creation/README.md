# Creation Components

Shared UI components for unified creation modals across all content types (gear, communities, requests, experiences).

## Overview

This directory contains reusable components extracted from the experience creation modal to enable consistent UX across all creation flows.

## Components

### InputModeToggle

Two-button toggle for switching between text and image input modes.

**Usage:**
```dart
InputModeToggle(
  currentMode: state.inputMode,
  onModeChanged: (mode) => notifier.setInputMode(mode),
  isLoading: state.isLoading,
)
```

**Features:**
- Material Design toggle pattern
- Haptic feedback on selection
- Disabled state during loading
- Orange primary color for selected state

### CameraViewport

Camera preview component with gallery picker integration.

**Usage:**
```dart
CameraViewport(
  selectedImagePath: state.selectedImagePath,
  onImageCaptured: (path) => notifier.setImagePath(path),
  onImageCleared: () => notifier.clearImage(),
  showGalleryButton: true,
)
```

**Features:**
- Auto-initializes camera on mount
- Shows selected image with clear button
- Gallery picker button in lower-left
- Loading and error states
- Image quality: 1920x1080 @ 85%

### TextInputArea

Multi-line text input for AI generation prompts.

**Usage:**
```dart
TextInputArea(
  controller: _textController,
  hintText: 'e.g., Potluck dinner this weekend...',
  onChanged: (value) => notifier.setTextInput(value),
  minLines: 4,
  maxLines: 4,
)
```

**Features:**
- Themed input field with app colors
- Configurable line counts
- Focus border on primary color
- Optional validation

## Existing Components to Reuse

The following components already exist and should be reused in creation modals:

### From `modal/`
- **ModalHelpers.showStandardModal()** - Standard bottom sheet with keyboard handling
- **ModalBuilders.buildGradientHeader()** - Orange gradient header
- **ModalBuilders.buildInfoDisplayRow()** - Info text display

### From `content/`
- **ContentEditableField** - Text inputs with overlay styling (28 tests)
- **ContentErrorBanner** - Inline error messages (8 tests)
- **ContentActionButton** - Full-width action buttons (15 tests)

### From `media/`
- **MediaPickerDialog** - Media source selection (camera/gallery)
- **MediaPickerHelper** - Consistent media quality settings

## Architecture Pattern

All creation modals should follow this pattern:

```
┌─────────────────────────────────────┐
│     Modal (showModalBottomSheet)     │
│                                      │
│  ┌────────────────────────────────┐ │
│  │    Gradient Header             │ │
│  │  (ModalBuilders)               │ │
│  └────────────────────────────────┘ │
│                                      │
│  ┌────────────────────────────────┐ │
│  │    Main Content Area           │ │
│  │  - CameraViewport (image mode) │ │
│  │  - TextInputArea (text mode)   │ │
│  └────────────────────────────────┘ │
│                                      │
│  ┌────────────────────────────────┐ │
│  │    Bottom Controls             │ │
│  │  - InputModeToggle             │ │
│  │  - Generate Button             │ │
│  └────────────────────────────────┘ │
└─────────────────────────────────────┘
```

## Modal Height Pattern

- **Text mode:** 60% screen height
- **Image mode:** 95% screen height
- **With keyboard:** Expands to 90% (handled by ModalHelpers)

## ViewModel Pattern

All creation ViewModels follow this structure:

```dart
@freezed
sealed class {Type}CreationState with _${Type}CreationState {
  const factory {Type}CreationState({
    @Default(CreationInputMode.text) CreationInputMode inputMode,
    @Default('') String textInput,
    String? selectedImagePath,
    @Default(false) bool isLoading,
    Gen{Type}Response? generatedData,
    String? errorMessage,
  }) = _{Type}CreationState;
}

class {Type}CreationNotifier extends Notifier<{Type}CreationState> {
  {Type}Repository get _repository => ref.read({type}RepositoryProvider);

  void setInputMode(CreationInputMode mode) { ... }
  void setTextInput(String text) { ... }
  void setImagePath(String path) { ... }
  void clearImage() { ... }
  bool canGenerate() { ... }
  Future<Gen{Type}Response?> generate{Type}() { ... }
  void reset() { ... }
}
```

## Testing

Components should have comprehensive unit tests:

- **InputModeToggle:** Mode switching, loading state, haptic feedback
- **CameraViewport:** Camera initialization, gallery picking, error states
- **TextInputArea:** Input handling, validation, styling

See test files in `test/presentation/widgets/creation/`

## Migration Checklist

When migrating a creation modal to use these components:

- [ ] Replace custom toggle with `InputModeToggle`
- [ ] Replace camera logic with `CameraViewport`
- [ ] Replace text input with `TextInputArea`
- [ ] Use `ModalBuilders.buildGradientHeader()` for header
- [ ] Use `ModalBuilders.buildInfoDisplayRow()` for instructions
- [ ] Update ViewModel to match standardized pattern
- [ ] Add unit tests
- [ ] Verify no regressions

## See Also

- [docs/ai/create2.md](../../../../../../docs/ai/create2.md) - Full migration plan
- [docs/client/modals.md](../../../../../../docs/client/modals.md) - Modal architecture
- [docs/client/content.md](../../../../../../docs/client/content.md) - Content components
