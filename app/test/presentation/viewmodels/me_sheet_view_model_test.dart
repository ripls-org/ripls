import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mockito/annotations.dart';
import 'package:mockito/mockito.dart';
import 'package:ripls/data/repositories/portfolio_repository.dart';
import 'package:ripls/presentation/viewmodels/me_sheet_view_model.dart';
import 'package:ripls/services/providers/feed_providers.dart'
    show portfolioRepositoryProvider;

import 'me_sheet_view_model_test.mocks.dart';

@GenerateMocks([PortfolioRepository])
void main() {
  late MockPortfolioRepository mockRepo;
  late ProviderContainer container;

  GetPortfolioMetricsResponse metrics(String costSaved) {
    return GetPortfolioMetricsResponse(
      communitiesTotal: PortfolioMetricsSection(costSaved: costSaved),
    );
  }

  setUp(() {
    mockRepo = MockPortfolioRepository();
    container = ProviderContainer(
      overrides: [
        portfolioRepositoryProvider.overrideWithValue(mockRepo),
      ],
    );
  });

  tearDown(() {
    container.dispose();
    reset(mockRepo);
  });

  /// Reads the provider's future while holding a listener (the provider
  /// is autoDispose; the mounted sheet is the real listener).
  Future<String?> readImpact() {
    container.listen(meSheetImpactProvider, (_, _) {});
    return container.read(meSheetImpactProvider.future);
  }

  test('resolves the server-formatted communities total', () async {
    when(mockRepo.getPortfolioMetrics(
            communityIds: anyNamed('communityIds')))
        .thenAnswer((_) async => metrics(r'$8,716'));

    expect(await readImpact(), r'$8,716');
  });

  test('resolves null for an empty or zero amount', () async {
    when(mockRepo.getPortfolioMetrics(
            communityIds: anyNamed('communityIds')))
        .thenAnswer((_) async => metrics(''));
    expect(await readImpact(), isNull);

    when(mockRepo.getPortfolioMetrics(
            communityIds: anyNamed('communityIds')))
        .thenAnswer((_) async => metrics(r'$0'));
    container.invalidate(meSheetImpactProvider);
    expect(await readImpact(), isNull);
  });

  test('failure surfaces as an error state, hiding the impact line',
      () async {
    when(mockRepo.getPortfolioMetrics(
            communityIds: anyNamed('communityIds')))
        .thenAnswer((_) async => throw Exception('boom'));

    container.listen(meSheetImpactProvider, (_, _) {});
    // Let the failing fetch settle.
    await Future<void>.delayed(const Duration(milliseconds: 10));

    final state = container.read(meSheetImpactProvider);
    expect(state.hasError, isTrue);
    // The sheet reads asData?.value — an error renders no line.
    expect(state.asData?.value, isNull);
  });
}
