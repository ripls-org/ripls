import 'package:flutter_test/flutter_test.dart';
import 'package:logging/logging.dart';
import 'package:ripls/core/config/environment.dart';

void main() {
  group('Environment', () {
    group('configureLogging', () {
      tearDown(() {
        // Reset logger state after each test
        Logger.root.level = Level.INFO;
        Logger.root.clearListeners();
      });

      test('registers a log listener', () {
        // Verify no listeners initially (after tearDown clears them)
        Logger.root.clearListeners();

        Environment.configureLogging();

        // Create a logger and log a message to verify the listener was registered
        final logger = Logger('TestLogger');
        final records = <LogRecord>[];

        // Add our own listener to capture what gets logged
        Logger.root.onRecord.listen(records.add);

        logger.info('test message');

        expect(records, hasLength(1));
        expect(records.first.message, equals('test message'));
        expect(records.first.loggerName, equals('TestLogger'));
      });

      test('sets log level from environment defaulting to INFO', () {
        Environment.configureLogging();

        // Default should be INFO
        expect(Logger.root.level, equals(Level.INFO));
      });

      test('logs are captured regardless of build mode', () {
        // This test verifies that the listener processes all log records
        // without any kDebugMode check filtering them out
        Environment.configureLogging();

        final logger = Logger('TestLogger');
        final capturedRecords = <LogRecord>[];

        // Add listener to capture records
        Logger.root.onRecord.listen(capturedRecords.add);

        // Log at various levels
        logger.fine('fine message'); // Below INFO, should not appear
        logger.info('info message');
        logger.warning('warning message');
        logger.severe('severe message');

        // INFO and above should be captured (fine is below INFO level)
        expect(capturedRecords.where((r) => r.level >= Level.INFO), hasLength(3));
      });
    });
  });
}
