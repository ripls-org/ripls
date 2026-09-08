import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/services/providers/feed_providers.dart'
    show portfolioRepositoryProvider;

/// meSheetImpactProvider resolves the Me sheet's identity impact line
/// (#2634 v2): the combined cost saved across all of the user's
/// communities, as the server-formatted amount (e.g. "$8,716"). Resolves
/// to null when there is nothing meaningful to show — the sheet then
/// renders no impact line rather than a zero.
final meSheetImpactProvider = FutureProvider.autoDispose<String?>((ref) async {
  final metrics =
      await ref.watch(portfolioRepositoryProvider).getPortfolioMetrics();
  final amount = metrics.communitiesTotal.costSaved.trim();
  if (amount.isEmpty || amount == r'$0') return null;
  return amount;
});
