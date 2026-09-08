import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:freezed_annotation/freezed_annotation.dart';

part 'portfolio_view_model.freezed.dart';

/// State for the Portfolio screen.
@freezed
sealed class PortfolioState with _$PortfolioState {
  const factory PortfolioState({
    @Default(true) bool isNavVisible,
  }) = _PortfolioState;
}

/// Notifier for managing the Portfolio screen state.
class PortfolioNotifier extends Notifier<PortfolioState> {
  @override
  PortfolioState build() {
    return const PortfolioState();
  }

  /// showNav makes the floating header and bottom nav visible.
  void showNav() {
    if (!state.isNavVisible) {
      state = state.copyWith(isNavVisible: true);
    }
  }

  /// hideNav hides the floating header and bottom nav.
  void hideNav() {
    if (state.isNavVisible) {
      state = state.copyWith(isNavVisible: false);
    }
  }

  /// toggleNav flips the nav visibility.
  void toggleNav() {
    state = state.copyWith(isNavVisible: !state.isNavVisible);
  }
}

/// portfolioViewModelProvider manages portfolio screen nav visibility state.
final portfolioViewModelProvider =
    NotifierProvider<PortfolioNotifier, PortfolioState>(PortfolioNotifier.new);
