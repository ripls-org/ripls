/// EquivalenceMetric identifies which ladder a value belongs to. The
/// string values are the analytics `metric` parameter on
/// `tier_unlocked` events — keep them stable.
enum EquivalenceMetric {
  timeTogether('time_together'),
  moneySaved('money_saved'),
  co2Avoided('co2_avoided');

  final String analyticsName;
  const EquivalenceMetric(this.analyticsName);
}

/// EquivalenceSurface identifies *where* a ladder is being rendered.
/// Used as a dedup key so the `tier_unlocked` analytics event fires
/// at most once per `(surface, metric, tier)` tuple. The string is
/// part of the persisted SharedPreferences key — never rename.
enum EquivalenceSurface {
  workshopDetail('workshop_detail'),
  profileRow('profile_row'),
  communityStage('community_stage');

  final String persistedName;
  const EquivalenceSurface(this.persistedName);
}
