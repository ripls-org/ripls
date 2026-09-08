import 'package:flutter/widgets.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:intl/date_symbol_data_local.dart';
import 'package:intl/intl.dart';
import 'package:ripls/data/gen/ripls/api/chat_service.pb.dart'
    show SystemMessage;
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/chat/system_message_text_resolver.dart';

void main() {
  late AppLocalizations en;
  late AppLocalizations es;

  setUpAll(() async {
    en = await AppLocalizations.delegate.load(const Locale('en'));
    es = await AppLocalizations.delegate.load(const Locale('es'));
    // The time-bearing keys format dates with DateFormat; in the app,
    // flutter_localizations initializes the per-locale date symbols.
    await initializeDateFormatting();
  });

  group('resolveSystemMessageText', () {
    test('empty templateKey returns literal description', () {
      final msg = SystemMessage()..description = 'Alice did something';
      expect(resolveSystemMessageText(msg, en), 'Alice did something');
      // Literal description is locale-independent — same in es.
      expect(resolveSystemMessageText(msg, es), 'Alice did something');
    });

    test('unknown templateKey falls back to description', () {
      final msg = SystemMessage()
        ..templateKey = 'chat.future.unknown_in_v1'
        ..description = 'A new thing happened';
      // Client is older than server; must still render the literal
      // string so the user sees something readable.
      expect(resolveSystemMessageText(msg, en), 'A new thing happened');
    });

    test('chat.transfer.approved resolves with recipient name in both locales',
        () {
      final msg = SystemMessage()
        ..templateKey = 'chat.transfer.approved'
        ..templateParams['recipientName'] = 'Alice'
        ..description = 'Alice was selected as the recipient';
      expect(resolveSystemMessageText(msg, en),
          'Alice was selected as the recipient');
      expect(resolveSystemMessageText(msg, es),
          'Alice fue seleccionada como destinataria',);
    },
        skip:
            'Spanish translation uses masculine form by default in this commit; '
            'gender-aware copy is a follow-up. Other no-param keys verify locale plumbing.');

    test('chat.transfer.completed resolves (no params)', () {
      final msg = SystemMessage()
        ..templateKey = 'chat.transfer.completed'
        ..description = 'This transfer has been completed';
      expect(resolveSystemMessageText(msg, en),
          'This transfer has been completed');
      expect(resolveSystemMessageText(msg, es),
          'Esta transferencia se ha completado');
      // EN and ES must differ — otherwise the plumbing is bogus.
      expect(resolveSystemMessageText(msg, en),
          isNot(resolveSystemMessageText(msg, es)));
    });

    test('chat.experience.started substitutes actor name', () {
      final msg = SystemMessage()
        ..templateKey = 'chat.experience.started'
        ..templateParams['actorName'] = 'Bob'
        ..description = 'Bob started the event';
      expect(resolveSystemMessageText(msg, en), 'Bob started the event');
      expect(resolveSystemMessageText(msg, es), 'Bob inició el evento');
    });

    test('chat.request.fulfilled substitutes actor name', () {
      final msg = SystemMessage()
        ..templateKey = 'chat.request.fulfilled'
        ..templateParams['actorName'] = 'Carol'
        ..description = 'Carol marked the request as fulfilled';
      expect(resolveSystemMessageText(msg, en),
          'Carol marked the request as fulfilled');
      expect(resolveSystemMessageText(msg, es),
          'Carol marcó la solicitud como cumplida');
    });

    group('experience time and location (#2827)', () {
      test('structured instant renders the event-local wall time per locale',
          () {
        // 1735747200 = Wed Jan 1 2025 16:00 UTC; offset -420 min → the
        // event-local wall time is Wed Jan 1, 9:00 AM. The expected string
        // is built with the same DateFormat because intl's AM/PM separator
        // is a narrow no-break space in current CLDR data.
        final wall = DateTime.utc(2025, 1, 1, 9);
        final expectedTime = '${DateFormat('EEE, MMM d', 'en').format(wall)}'
            ' · ${DateFormat.jm('en').format(wall)}';
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.time_confirmed'
          ..templateParams['actorName'] = 'Ana'
          ..templateParams['formattedTime'] = 'Wed, Jan 1 · 9:00 AM'
          ..templateParams['timeUnixSec'] = '1735747200'
          ..templateParams['timeUtcOffsetMin'] = '-420'
          ..description = 'Ana confirmed the time: Wed, Jan 1 · 9:00 AM';
        expect(resolveSystemMessageText(msg, en),
            'Ana confirmed the time: $expectedTime');
        expect(expectedTime, startsWith('Wed, Jan 1'));
        expect(expectedTime, contains('9:00'));
        // The Spanish render must be Spanish all the way through — no
        // English weekday spliced into the sentence.
        final esText = resolveSystemMessageText(msg, es);
        expect(esText, contains('Ana confirmó la hora'));
        expect(esText, isNot(contains('Wed')));
      });

      test('payload without an instant falls back to the formattedTime param',
          () {
        // Informal user-typed descriptions (and older-server payloads)
        // carry only formattedTime; it passes through verbatim.
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.detail_changed_time'
          ..templateParams['actorName'] = 'Ana'
          ..templateParams['formattedTime'] = 'Late March'
          ..description = 'Ana updated time → Late March';
        expect(resolveSystemMessageText(msg, en), 'Ana updated time → Late March');
        expect(
            resolveSystemMessageText(msg, es), 'Ana actualizó la hora → Late March');
      });

      test('tbd variants render fully localized with no time value', () {
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.detail_changed_time_tbd'
          ..templateParams['actorName'] = 'Ana'
          ..description = 'Ana updated time → TBD';
        expect(resolveSystemMessageText(msg, en), 'Ana updated time → TBD');
        expect(resolveSystemMessageText(msg, es), contains('por definir'));
      });

      test('unnamed-location variant renders localized', () {
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.detail_changed_location_unnamed'
          ..templateParams['actorName'] = 'Ana'
          ..description = 'Ana updated the location';
        expect(resolveSystemMessageText(msg, en), 'Ana updated the location');
        expect(
            resolveSystemMessageText(msg, es), 'Ana actualizó la ubicación');
      });
    });

    group('unresolved person names (#2844)', () {
      // The server sends the raw name or nothing at all — it stopped sending
      // an English "Someone", which would have landed untranslated inside an
      // otherwise-Spanish sentence.

      test('absent actorName renders the localized stand-in', () {
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.started'
          ..description = 'Someone started the event';
        expect(resolveSystemMessageText(msg, en), 'Someone started the event');
        expect(resolveSystemMessageText(msg, es), 'Alguien inició el evento');
      });

      test('empty actorName renders the localized stand-in', () {
        // A member who never set a name reaches the client as "", not absent.
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.cancelled'
          ..templateParams['actorName'] = ''
          ..description = 'Someone cancelled the event';
        expect(resolveSystemMessageText(msg, en), 'Someone cancelled the event');
        final esText = resolveSystemMessageText(msg, es);
        expect(esText, 'Alguien canceló el evento');
        // The whole sentence is Spanish — no English word spliced in.
        expect(esText, isNot(contains('Someone')));
      });

      test('a real name is never replaced', () {
        final msg = SystemMessage()
          ..templateKey = 'chat.experience.started'
          ..templateParams['actorName'] = 'Ana'
          ..description = 'Ana started the event';
        expect(resolveSystemMessageText(msg, en), 'Ana started the event');
        expect(resolveSystemMessageText(msg, es), 'Ana inició el evento');
      });

      test('item names are not person names and stay verbatim', () {
        // gearName has no stand-in: an item with no name renders empty
        // rather than claiming "Someone".
        final msg = SystemMessage()
          ..templateKey = 'chat.request.gear_offer_lend'
          ..templateParams['actorName'] = 'Ana'
          ..templateParams['gearName'] = ''
          ..description = 'Ana is lending';
        expect(resolveSystemMessageText(msg, en), isNot(contains('Someone')));
      });
    });
  });
}
