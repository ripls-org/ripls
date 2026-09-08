import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/data/gen/ripls/api/location.pb.dart'
    show Location, LocationProposal, ProposedLocation;
import 'package:ripls/data/repositories/location_repository.dart';
import 'package:ripls/presentation/viewmodels/location_modal_view_model.dart';
import 'package:ripls/services/providers.dart';

/// A location-poll proposal with its display name, address, and coordinates
/// already resolved — so the location panel and its map can read them without
/// calling a repository from the widget layer (client architecture rule: only
/// the view-model layer touches repositories).
class ResolvedLocationProposal {
  /// The underlying proposal (carries id, votes, proposedBy, pollId).
  final LocationProposal proposal;

  /// Human-readable place name (e.g. "Chautauqua Park").
  final String name;

  /// Secondary address line, when available.
  final String? address;

  /// Coordinates, when known. Null for a proposal whose saved location could
  /// not be resolved; `(0, 0)` is treated as "no usable coordinate" and
  /// dropped from the map marker set by the panel.
  final double? latitude;
  final double? longitude;

  const ResolvedLocationProposal({
    required this.proposal,
    required this.name,
    this.address,
    this.latitude,
    this.longitude,
  });

  /// Whether this proposal has a real coordinate worth plotting on the map.
  bool get hasCoordinate =>
      latitude != null &&
      longitude != null &&
      !(latitude == 0 && longitude == 0);
}

/// Resolved geography for the location panel: every proposal plotted on the
/// map / vote list, plus the single confirmed location (when one is set), each
/// with name + address + coordinates pre-resolved.
class LocationPanelGeo {
  final List<ResolvedLocationProposal> proposals;

  /// The confirmed event location, resolved for the "set" state. Null when no
  /// location has been confirmed.
  final ResolvedLocationProposal? confirmed;

  const LocationPanelGeo({required this.proposals, this.confirmed});
}

/// Resolves an experience's location-poll proposals (and the confirmed
/// location, if any) into [ResolvedLocationProposal]s — name + address +
/// coordinates — fetching saved `Location` records through the cached
/// repository. Re-resolves whenever the underlying [locationModalProvider]
/// state changes (new proposal, vote, confirm, etc.).
///
/// This lives in the view-model layer on purpose: the location panel reads the
/// resolved data and never calls `LocationRepository` itself.
final locationPanelGeoProvider =
    FutureProvider.family<LocationPanelGeo, String>((ref, experienceId) async {
  final data = await ref.watch(locationModalProvider(experienceId).future);
  final repo = ref.read(locationRepositoryProvider);

  final proposals = await Future.wait(
    data.proposals.map((p) => _resolveProposal(repo, p)),
  );

  // Resolve the confirmed location for the "set" state. Prefer a matching
  // proposal (already resolved); only fetch standalone when a location was set
  // via a plain edit that created no proposal.
  ResolvedLocationProposal? confirmed;
  final confirmedId = data.eventLocationId;
  if (confirmedId != null && confirmedId.isNotEmpty) {
    confirmed = proposals
        .where((r) =>
            r.proposal.location.locationId == confirmedId ||
            r.proposal.id == data.lockedProposalId)
        .firstOrNull;
    if (confirmed == null) {
      try {
        final saved = await repo.get(confirmedId);
        confirmed = ResolvedLocationProposal(
          proposal: LocationProposal()
            ..location = (ProposedLocation()..locationId = confirmedId),
          name: _savedName(saved) ?? confirmedId,
          address: _savedAddress(saved),
          latitude: saved.latitudeDeg,
          longitude: saved.longitudeDeg,
        );
      } catch (_) {
        // Leave confirmed null — the panel falls back to its no-coordinate UI.
      }
    }
  }

  return LocationPanelGeo(proposals: proposals, confirmed: confirmed);
});

Future<ResolvedLocationProposal> _resolveProposal(
  LocationRepository repo,
  LocationProposal p,
) async {
  final loc = p.location;
  if (loc.locationId.isNotEmpty) {
    try {
      final saved = await repo.get(loc.locationId);
      return ResolvedLocationProposal(
        proposal: p,
        name: _savedName(saved) ?? loc.locationId,
        address: _savedAddress(saved),
        latitude: saved.latitudeDeg,
        longitude: saved.longitudeDeg,
      );
    } catch (_) {
      // Resolution failure must not drop the proposal from the vote list —
      // fall back to the id as the label and leave it off the map.
      return ResolvedLocationProposal(proposal: p, name: loc.locationId);
    }
  }
  if (loc.hasGeocoded()) {
    final g = loc.geocoded;
    final name = g.name.isNotEmpty
        ? g.name
        : (g.addressLines.isNotEmpty ? g.addressLines.first : g.locality);
    final addressParts = <String>[];
    if (g.addressLines.isNotEmpty && g.name.isNotEmpty) {
      addressParts.add(g.addressLines.first);
    }
    if (g.locality.isNotEmpty) addressParts.add(g.locality);
    return ResolvedLocationProposal(
      proposal: p,
      name: name,
      address: addressParts.isEmpty ? null : addressParts.join(', '),
      latitude: g.latitudeDeg,
      longitude: g.longitudeDeg,
    );
  }
  return ResolvedLocationProposal(proposal: p, name: '—');
}

String? _savedName(Location saved) {
  if (saved.hasName() && saved.name.isNotEmpty) return saved.name;
  if (saved.addressLines.isNotEmpty) return saved.addressLines.first;
  if (saved.locality.isNotEmpty) return saved.locality;
  return null;
}

String? _savedAddress(Location saved) {
  final parts = <String>[];
  if (saved.addressLines.isNotEmpty && saved.hasName()) {
    parts.add(saved.addressLines.first);
  }
  if (saved.locality.isNotEmpty) parts.add(saved.locality);
  return parts.isEmpty ? null : parts.join(', ');
}
