import 'package:flutter/material.dart';

/// Deterministic per-community color helper. Maps a community id to one of
/// a small parchment-friendly palette so the Workshop CTA-row chip and
/// the Season Report breakdown bullets render with stable, varied dots
/// across runs without forcing a `color` field onto the wire.
///
/// The palette mirrors the reference design's circle hues
/// (rust-orange / olive-green / muted purple / earth-brown). Same id
/// always returns the same color — no per-session reshuffling.
const _communityPalette = <Color>[
  Color(0xFFA36D3A), // rust-orange (Backcountry-style)
  Color(0xFF5D7A4F), // olive-green (Horizons-style)
  Color(0xFF7A5A8A), // muted purple (Family-style)
  Color(0xFF7A5A4A), // earth-brown (Block-style)
  Color(0xFF6B7A5A), // dusty olive
  Color(0xFFC48A5B), // warm clay
];

/// Returns a stable color for `communityId`. Empty / null id falls back
/// to a neutral accent so the chip is still visible.
Color colorForCommunity(String? communityId) {
  if (communityId == null || communityId.isEmpty) {
    return _communityPalette[0];
  }
  // FNV-1a 32-bit hash — stable, fast, no allocations beyond the int.
  int hash = 2166136261;
  for (final code in communityId.codeUnits) {
    hash = (hash ^ code) & 0xFFFFFFFF;
    hash = (hash * 16777619) & 0xFFFFFFFF;
  }
  return _communityPalette[hash % _communityPalette.length];
}
