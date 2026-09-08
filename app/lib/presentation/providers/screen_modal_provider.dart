import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Notifier for managing screen-level modal state
class ScreenModalNotifier extends Notifier<Widget?> {
  @override
  Widget? build() => null;
  }

/// Provider for managing screen-level modals that need to render above
/// the bottom navigation bar.
///
/// Screens can inject modal content via this provider, and the parent
/// layout (HomeScreen) will render it in a dedicated modal layer.
///
/// This approach solves the Overlay rebuild issue by keeping the modal
/// in the widget tree while still rendering it above the bottom navigation
/// bar in z-space.
final screenModalProvider = NotifierProvider<ScreenModalNotifier, Widget?>(
  ScreenModalNotifier.new,
);
