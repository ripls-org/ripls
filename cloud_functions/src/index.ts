// Entry point for the ripls-alerts Functions codebase. Exports the deployed
// trigger handlers so the Firebase CLI can discover them. Implementation
// modules are split per signal source (crashlytics, gcp-alerts) and share
// the upsertIssue helper.
//
// Design: docs/issues/1148-1149-alert-to-issue.md

export {
  crashlyticsVelocityAlert,
  crashlyticsRegressionAlert,
  crashlyticsStabilityDigest,
} from './crashlyticsHandler';

export { gcpAlertsHandler } from './gcpAlertsHandler';

export { dailyGcpPolicySweep } from './dailySweepHandler';
