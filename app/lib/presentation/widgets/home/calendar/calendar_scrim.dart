import 'package:flutter/material.dart';

/// CalendarScrim is the full-bleed readability gradient over the calendar
/// backdrop — darker at the top (for the header) and bottom (for the day
/// detail), lighter through the grid. Shared by the inbox calendar screen and
/// the community calendar so both read identically over any photo.
class CalendarScrim extends StatelessWidget {
  const CalendarScrim({super.key});

  @override
  Widget build(BuildContext context) {
    return const DecoratedBox(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color(0xA3080C06),
            Color(0x57080C06),
            Color(0x61070B06),
            Color(0xED060A05),
          ],
          stops: [0.0, 0.30, 0.55, 1.0],
        ),
      ),
    );
  }
}
