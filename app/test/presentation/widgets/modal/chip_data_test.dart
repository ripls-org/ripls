import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:ripls/presentation/widgets/modal/chip_data.dart';

void main() {
  group('ChipData', () {
    test('creates instance with required fields', () {
      final chip = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
      );

      expect(chip.label, 'Home');
      expect(chip.icon, Icons.home);
      expect(chip.value, 'home');
      expect(chip.isPrimary, false);
    });

    test('creates instance with isPrimary flag', () {
      final chip = ChipData(
        label: 'Work',
        icon: Icons.work,
        value: 'work',
        isPrimary: true,
      );

      expect(chip.isPrimary, true);
    });

    test('equality works correctly', () {
      final chip1 = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
      );

      final chip2 = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
      );

      final chip3 = ChipData(
        label: 'Work',
        icon: Icons.work,
        value: 'work',
      );

      expect(chip1, equals(chip2));
      expect(chip1, isNot(equals(chip3)));
    });

    test('equality includes isPrimary flag', () {
      final chip1 = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
        isPrimary: true,
      );

      final chip2 = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
        isPrimary: false,
      );

      expect(chip1, isNot(equals(chip2)));
    });

    test('hashCode works correctly', () {
      final chip1 = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
      );

      final chip2 = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
      );

      expect(chip1.hashCode, equals(chip2.hashCode));
    });

    test('toString includes relevant information', () {
      final chip = ChipData(
        label: 'Home',
        icon: Icons.home,
        value: 'home',
        isPrimary: true,
      );

      final stringRep = chip.toString();
      expect(stringRep, contains('Home'));
      expect(stringRep, contains('home'));
      expect(stringRep, contains('true'));
    });

    test('can be used in collections', () {
      final chips = [
        ChipData(
          label: 'Home',
          icon: Icons.home,
          value: 'home',
          isPrimary: true,
        ),
        ChipData(
          label: 'Work',
          icon: Icons.work,
          value: 'work',
        ),
        ChipData(
          label: 'Gym',
          icon: Icons.fitness_center,
          value: 'gym',
        ),
      ];

      expect(chips.length, 3);
      expect(chips[0].isPrimary, true);
      expect(chips[1].isPrimary, false);
    });

    test('can find chip by value', () {
      final chips = [
        ChipData(label: 'Home', icon: Icons.home, value: 'home'),
        ChipData(label: 'Work', icon: Icons.work, value: 'work'),
      ];

      final workChip = chips.firstWhere((chip) => chip.value == 'work');
      expect(workChip.label, 'Work');
    });

    test('icon is now optional (glass-chip variant)', () {
      final chip = ChipData(value: 'today', primary: 'Today');
      expect(chip.icon, isNull);
      expect(chip.label, '');
    });

    test('primaryText falls back to label when primary is null', () {
      final chip = ChipData(label: 'Home', icon: Icons.home, value: 'home');
      expect(chip.primaryText, 'Home');
    });

    test('primaryText prefers primary over label', () {
      final chip = ChipData(
        label: 'fallback',
        value: 'today',
        primary: 'Today',
      );
      expect(chip.primaryText, 'Today');
    });

    test('secondary line is captured', () {
      final chip = ChipData(
        value: 'today',
        primary: 'Today',
        secondary: 'May 8',
      );
      expect(chip.secondary, 'May 8');
    });

    test('equality includes primary and secondary fields', () {
      final a = ChipData(value: 'x', primary: 'P', secondary: 'S');
      final b = ChipData(value: 'x', primary: 'P', secondary: 'S');
      final c = ChipData(value: 'x', primary: 'P', secondary: 'different');
      expect(a, equals(b));
      expect(a, isNot(equals(c)));
    });
  });
}
