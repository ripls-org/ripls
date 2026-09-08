import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_theme.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/workshop/workshop_overview_theme.dart';

/// The metrics prose sentence with four underlined, tappable numbers (problems,
/// money, hours, CO₂) that morph-open their deep dives. The deep dives grow from
/// the whole sentence's footprint. Extracted from the Workshop overview to keep
/// that file under the Dart line-count gate.
class WorkshopMetricsSentence extends StatefulWidget {
  final int hours;
  final int dollars;
  final double co2Kg;
  final int problems;
  final int problemsPotential;
  final int memberCount;
  final ValueChanged<Rect> onTapTime;
  final ValueChanged<Rect> onTapMoney;
  final ValueChanged<Rect> onTapCo2;
  final ValueChanged<Rect> onTapProblems;

  const WorkshopMetricsSentence({
    super.key,
    required this.hours,
    required this.dollars,
    required this.co2Kg,
    required this.problems,
    required this.problemsPotential,
    required this.memberCount,
    required this.onTapTime,
    required this.onTapMoney,
    required this.onTapCo2,
    required this.onTapProblems,
  });

  @override
  State<WorkshopMetricsSentence> createState() =>
      _WorkshopMetricsSentenceState();
}

class _WorkshopMetricsSentenceState extends State<WorkshopMetricsSentence> {
  final _key = GlobalKey();
  late final TapGestureRecognizer _time;
  late final TapGestureRecognizer _money;
  late final TapGestureRecognizer _co2;
  late final TapGestureRecognizer _problems;

  @override
  void initState() {
    super.initState();
    _time = TapGestureRecognizer()..onTap = () => _fire(widget.onTapTime);
    _money = TapGestureRecognizer()..onTap = () => _fire(widget.onTapMoney);
    _co2 = TapGestureRecognizer()..onTap = () => _fire(widget.onTapCo2);
    _problems = TapGestureRecognizer()
      ..onTap = () => _fire(widget.onTapProblems);
  }

  @override
  void dispose() {
    _time.dispose();
    _money.dispose();
    _co2.dispose();
    _problems.dispose();
    super.dispose();
  }

  void _fire(ValueChanged<Rect> cb) {
    final ctx = _key.currentContext;
    if (ctx != null) cb(workshopRectOf(ctx));
  }

  static String _commas(int n) {
    final s = n.abs().toString();
    final buf = StringBuffer();
    for (var i = 0; i < s.length; i++) {
      if (i > 0 && (s.length - i) % 3 == 0) buf.write(',');
      buf.write(s[i]);
    }
    return '${n < 0 ? '-' : ''}$buf';
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final kg = widget.co2Kg.round();
    const base = TextStyle(
      fontFamily: AppTheme.headingFont,
      fontSize: 14.5,
      height: 1.5,
      color: WorkshopOverviewPalette.onPhoto,
    );
    const link = TextStyle(
      decoration: TextDecoration.underline,
      decorationColor: Color(0x80FFFFFF),
      color: WorkshopOverviewPalette.onPhoto,
    );
    const nudgeStyle = TextStyle(color: WorkshopOverviewPalette.onPhotoDim);

    // Assemble the sentence from only the metrics that are above zero — a lone
    // $0 or 0 kg never renders inline, since one zero next to real numbers
    // reads as broken. Each clause is a verb word + a tappable value.
    final clauses = <List<InlineSpan>>[];

    if (widget.problems > 0) {
      // "X of Y problems" while requests are still open; plain "X problems" once
      // every request has been handled (avoids an awkward "5 of 5").
      final valueText = widget.problems < widget.problemsPotential
          ? l10n.workshopMetricsProblemsValue(
              _commas(widget.problems),
              _commas(widget.problemsPotential),
            )
          : l10n.workshopMetricsProblemsValuePlain(
              widget.problems,
              _commas(widget.problems),
            );
      clauses.add([
        TextSpan(text: '${l10n.workshopMetricsVerbSolved} '),
        TextSpan(text: valueText, style: link, recognizer: _problems),
      ]);
    }
    if (widget.dollars > 0) {
      clauses.add([
        TextSpan(text: '${l10n.workshopMetricsVerbSaved} '),
        TextSpan(
          text: '\$${_commas(widget.dollars)}',
          style: link,
          recognizer: _money,
        ),
      ]);
    }
    if (widget.hours > 0) {
      clauses.add([
        TextSpan(text: '${l10n.workshopMetricsVerbShared} '),
        TextSpan(
          text: l10n.workshopMetricsHoursValue(
            widget.hours,
            _commas(widget.hours),
          ),
          style: link,
          recognizer: _time,
        ),
      ]);
    }
    if (kg > 0) {
      clauses.add([
        TextSpan(text: '${l10n.workshopMetricsVerbKept} '),
        TextSpan(text: '${_commas(kg)} kg', style: link, recognizer: _co2),
        // Suffix shares the CO₂ tap target but stays un-underlined.
        TextSpan(text: l10n.workshopMetricsCo2Suffix, recognizer: _co2),
      ]);
    }

    final spans = <InlineSpan>[];
    if (clauses.isNotEmpty) {
      spans.add(TextSpan(
        text: widget.memberCount == 1
            ? l10n.workshopMetricsSentenceLeadSolo
            : l10n.workshopMetricsSentenceLead,
      ));
      spans.addAll(_joinClauses(l10n, clauses));
      spans.add(const TextSpan(text: '.'));

      // At most one trailing nudge: reframe the zero-solved problems as an
      // invitation, or turn a single lagging metric into a prompt.
      final trailing = _trailing(l10n, kg, nudgeStyle);
      if (trailing != null) {
        spans.add(const TextSpan(text: ' '));
        spans.addAll(trailing);
      }
    } else {
      // No verb clauses — the only thing to show is the open-requests reframe
      // (the parent withholds this widget entirely when even that is zero).
      final reframe = _openRequestsReframe(l10n);
      if (reframe != null) spans.addAll(reframe);
    }

    return Text.rich(
      key: _key,
      textAlign: TextAlign.center,
      TextSpan(style: base, children: spans),
    );
  }

  /// Joins clause span-lists with commas and a grammatical conjunction before
  /// the last (Oxford "and" for three or more).
  List<InlineSpan> _joinClauses(
    AppLocalizations l10n,
    List<List<InlineSpan>> clauses,
  ) {
    final out = <InlineSpan>[];
    for (var i = 0; i < clauses.length; i++) {
      if (i > 0) {
        final String sep;
        if (clauses.length == 2) {
          sep = l10n.workshopMetricsClauseAnd;
        } else if (i == clauses.length - 1) {
          sep = l10n.workshopMetricsClauseCommaAnd;
        } else {
          sep = l10n.workshopMetricsClauseSeparator;
        }
        out.add(TextSpan(text: sep));
      }
      out.addAll(clauses[i]);
    }
    return out;
  }

  /// The "N open requests waiting on a hand." reframe (tappable to the problems
  /// deep-dive). Null unless nothing is solved yet but requests are open — this is
  /// what replaces the demoralizing "0 of N" clause.
  List<InlineSpan>? _openRequestsReframe(AppLocalizations l10n) {
    if (widget.problems > 0 || widget.problemsPotential <= 0) return null;
    return [
      TextSpan(
        text: l10n.workshopMetricsOpenRequests(
          widget.problemsPotential,
          _commas(widget.problemsPotential),
        ),
        recognizer: _problems,
      ),
    ];
  }

  /// At most one trailing sentence for the metric line: the open-requests
  /// reframe, otherwise a soft nudge when exactly one metric lags an
  /// otherwise-healthy crew. Only money has bespoke copy today; other
  /// single-laggards stay hidden rather than shipping unreviewed wording.
  List<InlineSpan>? _trailing(
    AppLocalizations l10n,
    int kg,
    TextStyle nudgeStyle,
  ) {
    final reframe = _openRequestsReframe(l10n);
    if (reframe != null) return reframe;

    final laggards = [
      if (widget.problems == 0) 'problems',
      if (widget.dollars == 0) 'money',
      if (widget.hours == 0) 'hours',
      if (kg == 0) 'co2',
    ];
    if (laggards.length == 1 && laggards.first == 'money') {
      return [TextSpan(text: l10n.workshopMetricsNudgeMoney, style: nudgeStyle)];
    }
    return null;
  }
}
