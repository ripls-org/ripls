import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/core/utils/attribution_utils.dart';
import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';

void main() {
  group('AttributionUtils', () {
    group('getProviderName', () {
      test('returns "Unsplash" for Unsplash provider', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH,
        );

        expect(AttributionUtils.getProviderName(attribution), 'Unsplash');
      });

      test('returns "Pexels" for Pexels provider', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
        );

        expect(AttributionUtils.getProviderName(attribution), 'Pexels');
      });

      test('returns "Unknown" for unspecified provider', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPECIFIED,
        );

        expect(AttributionUtils.getProviderName(attribution), 'Unknown');
      });
    });

    group('getCreatorProfileUrl', () {
      test('constructs Unsplash profile URL from username', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH,
          creatorUsername: 'janesmith',
        );

        final url = AttributionUtils.getCreatorProfileUrl(attribution);
        expect(url, 'https://unsplash.com/@janesmith?utm_source=ripls&utm_medium=referral');
      });

      test('uses photographer URL for Pexels', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
          photographerUrl: 'https://www.pexels.com/@johndoe',
        );

        final url = AttributionUtils.getCreatorProfileUrl(attribution);
        expect(url, 'https://www.pexels.com/@johndoe?utm_source=ripls&utm_medium=referral');
      });

      test('returns empty string for unspecified provider', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPECIFIED,
        );

        expect(AttributionUtils.getCreatorProfileUrl(attribution), '');
      });
    });

    group('getPlatformUrl', () {
      test('uses original URL when available for Unsplash', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH,
          originalUrl: 'https://unsplash.com/photos/test123',
        );

        final url = AttributionUtils.getPlatformUrl(attribution);
        expect(url, 'https://unsplash.com/photos/test123?utm_source=ripls&utm_medium=referral');
      });

      test('uses original URL when available for Pexels', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
          originalUrl: 'https://www.pexels.com/photo/sunset-beach-12345678/',
        );

        final url = AttributionUtils.getPlatformUrl(attribution);
        expect(url, 'https://www.pexels.com/photo/sunset-beach-12345678/?utm_source=ripls&utm_medium=referral');
      });

      test('falls back to homepage when original URL is empty', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
          originalUrl: '',
        );

        final url = AttributionUtils.getPlatformUrl(attribution);
        expect(url, 'https://pexels.com?utm_source=ripls&utm_medium=referral');
      });
    });

    group('getProviderHomepage', () {
      test('returns Unsplash homepage', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH,
        );

        expect(AttributionUtils.getProviderHomepage(attribution), 'https://unsplash.com');
      });

      test('returns Pexels homepage', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS,
        );

        expect(AttributionUtils.getProviderHomepage(attribution), 'https://pexels.com');
      });

      test('returns empty string for unspecified provider', () {
        final attribution = Attribution(
          provider: StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPECIFIED,
        );

        expect(AttributionUtils.getProviderHomepage(attribution), '');
      });
    });
  });
}
