import 'package:ripls/data/gen/ripls/api/media_service.pb.dart';

/// Utility functions for stock image attribution.
///
/// Centralizes logic for building attribution URLs and display text
/// for supported stock image providers (Unsplash, Pexels).
class AttributionUtils {
  static const _utmParams = 'utm_source=ripls&utm_medium=referral';

  /// Returns the display name for the stock image provider.
  static String getProviderName(Attribution attribution) {
    switch (attribution.provider) {
      case StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH:
        return 'Unsplash';
      case StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS:
        return 'Pexels';
      default:
        return 'Unknown';
    }
  }

  /// Returns the URL to the creator's profile page with UTM parameters.
  static String getCreatorProfileUrl(Attribution attribution) {
    switch (attribution.provider) {
      case StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH:
        return 'https://unsplash.com/@${attribution.creatorUsername}?$_utmParams';
      case StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS:
        return '${attribution.photographerUrl}?$_utmParams';
      default:
        return '';
    }
  }

  /// Returns the URL to the photo page on the provider's platform with UTM parameters.
  static String getPlatformUrl(Attribution attribution) {
    final baseUrl = attribution.originalUrl.isNotEmpty
        ? attribution.originalUrl
        : getProviderHomepage(attribution);
    return '$baseUrl?$_utmParams';
  }

  /// Returns the homepage URL for the stock image provider.
  static String getProviderHomepage(Attribution attribution) {
    switch (attribution.provider) {
      case StockImageProvider.STOCK_IMAGE_PROVIDER_UNSPLASH:
        return 'https://unsplash.com';
      case StockImageProvider.STOCK_IMAGE_PROVIDER_PEXELS:
        return 'https://pexels.com';
      default:
        return '';
    }
  }
}
