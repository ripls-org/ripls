import { describe, expect, test, vi } from 'vitest';
import type { IssueTracker } from '../issueTracker';
import {
  AUTO_FILED_LABEL,
  CLAUDE_FIX_LABEL,
} from '../upsertIssue';
import {
  CloudMonitoringIncidentPayload,
  buildIncidentBody,
  buildResolutionBody,
  classifySeverity,
  handleGcpAlert,
  incidentDateUtc,
  parsePubsubMessage,
  policySlug,
  redactLabels,
} from '../gcpAlertsHandler';

const makeTracker = () => {
  const listForRepo = vi.fn().mockResolvedValue({ data: [] });
  const create = vi.fn().mockResolvedValue({ data: { number: 1234 } });
  const update = vi.fn().mockResolvedValue({ data: {} });
  const createComment = vi.fn().mockResolvedValue({ data: {} });
  const addLabels = vi.fn().mockResolvedValue({ data: {} });
  return {
    tracker: {
      issues: { listForRepo, create, update, createComment, addLabels },
    } as unknown as IssueTracker,
    spies: { listForRepo, create, update, createComment, addLabels },
  };
};

const baseIncident = (overrides: Partial<CloudMonitoringIncidentPayload['incident']> = {}): CloudMonitoringIncidentPayload => ({
  version: '1.2',
  incident: {
    incident_id: 'inc-7771',
    state: 'open',
    summary: 'Database connections - utilization is above 80%',
    started_at: 1747051200, // 2025-05-12T12:00:00Z
    policy_name: 'projects/ripls-prod/alertPolicies/9876543210',
    condition_name: 'projects/ripls-prod/alertPolicies/9876543210/conditions/abc',
    condition: { displayName: 'Database connections - utilization' },
    url: 'https://console.cloud.google.com/monitoring/alerting/incidents/inc-7771',
    severity: 'WARNING',
    resource: {
      type: 'cloudsql_database',
      labels: { project_id: 'ripls-prod', database_id: 'ripls-prod:gear-db-prod' },
    },
    metric: {
      type: 'cloudsql.googleapis.com/database/postgresql/num_backends',
      labels: {},
    },
    threshold_value: '40',
    observed_value: '45',
    ...overrides,
  },
});

describe('classifySeverity', () => {
  test('uptime check → page', () => {
    const p = baseIncident({ condition: { displayName: 'Uptime check failure' } });
    expect(classifySeverity(p)).toBe('page');
  });

  test('health check → page', () => {
    const p = baseIncident({ condition: { displayName: 'Health Check Unhealthy - server' } });
    expect(classifySeverity(p)).toBe('page');
  });

  test('database → page (any database alert)', () => {
    const p = baseIncident({ condition: { displayName: 'Database CPU - utilization' } });
    expect(classifySeverity(p)).toBe('page');
  });

  test('server error logged → warn', () => {
    const p = baseIncident({ condition: { displayName: 'Server Error Logged - server' } });
    expect(classifySeverity(p)).toBe('warn');
  });

  test('rpc errors → warn', () => {
    const p = baseIncident({ condition: { displayName: 'RPC Errors Elevated' } });
    expect(classifySeverity(p)).toBe('warn');
  });

  // The tests below pin the ACTUAL condition display_name strings from
  // terraform/modules/monitoring/{main,alerts,slos}.tf. #2622 found two
  // policies filing as "unknown" because the patterns had been written
  // against policy names while the classifier reads condition names — if a
  // Terraform rename stops matching, the failure here is the tripwire.

  test('all_errors actual condition string → warn', () => {
    const p = baseIncident({
      condition: {
        displayName: 'Service-level error logged (excluding middleware and infrastructure)',
      },
    });
    expect(classifySeverity(p)).toBe('warn');
  });

  test('rpc_error_rate actual condition string → warn', () => {
    const p = baseIncident({ condition: { displayName: 'RPC errors exceed 5% of requests' } });
    expect(classifySeverity(p)).toBe('warn');
  });

  test('p95 latency actual condition strings (both tiers) → warn', () => {
    for (const name of [
      'P95 latency above 2000ms for a non-streaming RPC',
      'P95 latency above 8000ms for a slow-class RPC',
    ]) {
      const p = baseIncident({ condition: { displayName: name } });
      expect(classifySeverity(p)).toBe('warn');
    }
  });

  test('SLO burn-rate condition strings → warn', () => {
    for (const name of [
      'Availability SLO burn rate - fast window (1h)',
      'Availability SLO burn rate - slow window (6h)',
      'Latency SLO burn rate - fast window (1h)',
      'Latency SLO burn rate - slow window (6h)',
    ]) {
      const p = baseIncident({ condition: { displayName: name } });
      expect(classifySeverity(p)).toBe('warn');
    }
  });

  test('outbound delivery condition strings → warn', () => {
    for (const name of [
      // sms_delivery_failure_rate (#2569), interpolated at 0.10.
      'SMS delivery failures exceed 10% of messages',
      // The email counterpart (#2862) will land on the same pattern.
      'Email delivery failures exceed 5% of sign-in codes',
    ]) {
      const p = baseIncident({ condition: { displayName: name } });
      expect(classifySeverity(p)).toBe('warn');
    }
  });

  // email_code_delivery_latency (#2862). Matches on /latency/i rather than a
  // delivery-specific pattern — same route the RPC p95 policies take — so this
  // pin is what catches a rename that drops the word "latency".
  test('email delivery latency actual condition string → warn', () => {
    const p = baseIncident({
      condition: { displayName: 'P95 delivery latency above 10000ms for sign-in codes' },
    });
    expect(classifySeverity(p)).toBe('warn');
  });

  // email_code_delivery_latency volume-floor condition (#2923). Must also match
  // /latency/i so the classifier routes it to warn, not unknown (#2622).
  test('email delivery latency floor condition string → warn', () => {
    const p = baseIncident({
      condition: { displayName: 'Sign-in code delivery latency: minimum sample floor of 5 per 10 min' },
    });
    expect(classifySeverity(p)).toBe('warn');
  });

  test('db pool saturation actual condition string → page', () => {
    const p = baseIncident({
      condition: { displayName: 'Database connection pool utilization above 90%' },
    });
    expect(classifySeverity(p)).toBe('page');
  });

  test('unknown policy → unknown', () => {
    const p = baseIncident({
      condition: { displayName: 'Some new policy we have not tagged' },
      summary: 'something',
    });
    expect(classifySeverity(p)).toBe('unknown');
  });

  test('falls back to summary when condition.displayName is missing', () => {
    const p = baseIncident({ condition: undefined, summary: 'Uptime check failure detected' });
    expect(classifySeverity(p)).toBe('page');
  });
});

describe('policySlug', () => {
  test('extracts numeric policy id from policy_name path', () => {
    const p = baseIncident();
    expect(policySlug(p)).toBe('9876543210');
  });

  test('falls back to slugified condition.displayName when no numeric id', () => {
    const p = baseIncident({
      policy_name: undefined,
      condition: { displayName: 'Server Error Logged - server' },
    });
    expect(policySlug(p)).toBe('server-error-logged-server');
  });

  test('returns "unknown" if neither policy id nor displayName present', () => {
    const p = baseIncident({
      policy_name: undefined,
      condition: undefined,
      summary: undefined,
    });
    expect(policySlug(p)).toBe('unknown');
  });

  // #1786: GitHub label names are limited to 50 chars. The full label is
  // `gcp-policy:<slug>-<yyyymmdd>` = 20 + slug. So slug must be ≤ 30 chars.
  test('caps displayName-fallback slug at 30 chars (#1786)', () => {
    const p = baseIncident({
      policy_name: undefined,
      condition: {
        displayName: 'Server Error Logged - smoke-test-2',
      },
    });
    const slug = policySlug(p);
    expect(slug.length).toBeLessThanOrEqual(30);
  });

  test('full gcp-policy label fits under 50 chars even for long displayName (#1786)', () => {
    const p = baseIncident({
      policy_name: undefined,
      condition: {
        displayName:
          'A very long Cloud Monitoring policy name that the operator gave a verbose label',
      },
    });
    const slug = policySlug(p);
    const label = `gcp-policy:${slug}-20260507`;
    expect(label.length).toBeLessThanOrEqual(50);
  });

  test('truncation does not leave dangling hyphen at the end of slug', () => {
    const p = baseIncident({
      policy_name: undefined,
      // Displayname is structured so a naive slice(0, 30) would land on a
      // hyphen: "abcdefghij-klmnopqrst-uvwxyz" — 28 chars, the next char
      // after slugify would be a hyphen.
      condition: { displayName: 'abcdefghij klmnopqrst uvwxyz hello-world' },
    });
    const slug = policySlug(p);
    expect(slug.endsWith('-')).toBe(false);
  });

  test('numeric policy id is also capped (defensive)', () => {
    // Real Cloud Monitoring IDs are ~19 digits and fit fine; this just
    // proves the cap applies on both branches in case Google ever changes
    // the ID shape.
    const p = baseIncident({
      policy_name:
        'projects/ripls-prod/alertPolicies/12345678901234567890123456789012345',
    });
    expect(policySlug(p).length).toBeLessThanOrEqual(30);
  });
});

describe('incidentDateUtc', () => {
  test('returns the YYYY-MM-DD of started_at in UTC', () => {
    // 1747051200 = 2025-05-12T12:00:00Z
    expect(incidentDateUtc(baseIncident({ started_at: 1747051200 }))).toBe('2025-05-12');
  });

  test('falls back to "today" when started_at is missing', () => {
    const today = new Date().toISOString().slice(0, 10);
    expect(incidentDateUtc(baseIncident({ started_at: undefined }))).toBe(today);
  });
});

describe('redactLabels', () => {
  test('drops fields whose name contains email/phone', () => {
    const out = redactLabels({
      user_id: 'u-1',
      user_email: 'alice@example.com',
      phone_number: '+15551234567',
      something_phone: 'masked-or-not',
      ok_field: 'value',
    });
    expect(out).toEqual({ user_id: 'u-1', ok_field: 'value' });
  });

  test('masks email-shaped values even if the field name is not "email"', () => {
    const out = redactLabels({ contact: 'bob@example.com' });
    expect(out.contact).toBe('bo***@example.com');
  });

  test('passes through non-email values unchanged', () => {
    const out = redactLabels({ request_id: 'req-abc-123' });
    expect(out.request_id).toBe('req-abc-123');
  });

  test('returns empty object on undefined input', () => {
    expect(redactLabels(undefined)).toEqual({});
  });
});

describe('buildIncidentBody', () => {
  test('includes incident id, state, started, policy, summary, threshold/observed', () => {
    const body = buildIncidentBody(baseIncident());
    expect(body).toContain('inc-7771');
    expect(body).toContain('open');
    expect(body).toContain('Database connections - utilization');
    expect(body).toContain('alertPolicies/9876543210');
    expect(body).toContain('40');
    expect(body).toContain('45');
  });

  test('redacts email-shaped values in resource labels', () => {
    const p = baseIncident({
      resource: {
        type: 'global',
        labels: { user_email: 'alice@example.com', request_id: 'r-9' },
      },
    });
    const body = buildIncidentBody(p);
    expect(body).not.toContain('alice@example.com');
    expect(body).not.toContain('user_email');
    expect(body).toContain('r-9');
  });

  test('omits the resource section when no labels remain after redaction', () => {
    const p = baseIncident({
      resource: { type: 'global', labels: { phone_number: '+1', user_email: 'a@b' } },
    });
    const body = buildIncidentBody(p);
    expect(body).not.toContain('Resource labels:');
  });
});

describe('buildResolutionBody', () => {
  test('includes incident id and ended_at when present', () => {
    const p = baseIncident({ state: 'closed', ended_at: 1747010000 });
    const body = buildResolutionBody(p);
    expect(body).toContain('inc-7771');
    expect(body).toContain('2025-05-12');
  });

  test('falls back to current time when ended_at missing', () => {
    const p = baseIncident({ state: 'closed', ended_at: null });
    const body = buildResolutionBody(p);
    expect(body).toMatch(/\d{4}-\d{2}-\d{2}T/);
  });
});

describe('handleGcpAlert — page severity', () => {
  test('open creates issue with severity:page + claude-fix labels', async () => {
    const { tracker, spies } = makeTracker();
    await handleGcpAlert(baseIncident(), tracker);

    expect(spies.create).toHaveBeenCalledTimes(1);
    const call = spies.create.mock.calls[0][0];
    expect(call.title).toContain('[ops:page]');
    expect(call.title).toContain('Database connections');
    const labels = call.labels as string[];
    expect(labels).toContain(AUTO_FILED_LABEL);
    expect(labels).toContain('gcp-incident:inc-7771');
    expect(labels).toContain('ops');
    expect(labels).toContain('severity:page');
    expect(labels).toContain(CLAUDE_FIX_LABEL);
  });

  test('open dedups against existing open issue (comments, no duplicate)', async () => {
    const { tracker, spies } = makeTracker();
    spies.listForRepo.mockResolvedValueOnce({
      data: [{ number: 88, state: 'open' }],
    });
    await handleGcpAlert(baseIncident(), tracker);

    expect(spies.create).not.toHaveBeenCalled();
    expect(spies.createComment).toHaveBeenCalledTimes(1);
  });

  test('closed → closeIssue path (comment + close on the matching issue)', async () => {
    const { tracker, spies } = makeTracker();
    spies.listForRepo.mockResolvedValueOnce({
      data: [{ number: 88, state: 'open' }],
    });
    await handleGcpAlert(baseIncident({ state: 'closed', ended_at: 1747010000 }), tracker);

    expect(spies.update).toHaveBeenCalledWith(
      expect.objectContaining({ issue_number: 88, state: 'closed' }),
    );
    expect(spies.createComment).toHaveBeenCalledTimes(1);
  });

  test('closed → noop when no open issue exists (incident already resolved)', async () => {
    const { tracker, spies } = makeTracker();
    await handleGcpAlert(baseIncident({ state: 'closed' }), tracker);

    expect(spies.update).not.toHaveBeenCalled();
    expect(spies.createComment).not.toHaveBeenCalled();
  });

  test('surfaces dependency/backend metric labels as dep:/backend: labels (health check)', async () => {
    const { tracker, spies } = makeTracker();
    await handleGcpAlert(
      baseIncident({
        condition: { displayName: 'Health Check Unhealthy - server' },
        metric: { type: 'health/status', labels: { dependency: 'ai', backend: 'anthropic' } },
      }),
      tracker,
    );
    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).toContain('dep:ai');
    expect(labels).toContain('backend:anthropic');
  });
});

describe('handleGcpAlert — warn severity', () => {
  test('open files with daily aggregation key and severity:warn label', async () => {
    const { tracker, spies } = makeTracker();
    await handleGcpAlert(
      baseIncident({
        condition: { displayName: 'Server Error Logged - server' },
        started_at: 1747051200, // 2025-05-12
      }),
      tracker,
    );

    const call = spies.create.mock.calls[0][0];
    expect(call.title).toContain('[ops:warn]');
    expect(call.title).toContain('2025-05-12');
    const labels = call.labels as string[];
    expect(labels).toContain('severity:warn');
    expect(labels.some((l) => /^gcp-policy:.+-20250512$/.test(l))).toBe(true);
  });

  test('two warn incidents same day → second is comment, not new issue', async () => {
    const { tracker, spies } = makeTracker();
    const day1A = baseIncident({
      incident_id: 'inc-A',
      condition: { displayName: 'Server Error Logged - server' },
      started_at: 1747051200,
    });
    const day1B = baseIncident({
      incident_id: 'inc-B',
      condition: { displayName: 'Server Error Logged - server' },
      started_at: 1747052200,
    });
    spies.listForRepo
      .mockResolvedValueOnce({ data: [] })
      .mockResolvedValueOnce({ data: [{ number: 200, state: 'open' }] });

    await handleGcpAlert(day1A, tracker);
    await handleGcpAlert(day1B, tracker);

    expect(spies.create).toHaveBeenCalledTimes(1);
    expect(spies.createComment).toHaveBeenCalledTimes(1);
  });

  test('warn closed events are dropped (daily aggregation; closes via daily sweep)', async () => {
    const { tracker, spies } = makeTracker();
    await handleGcpAlert(
      baseIncident({
        condition: { displayName: 'Server Error Logged - server' },
        state: 'closed',
      }),
      tracker,
    );
    expect(spies.create).not.toHaveBeenCalled();
    expect(spies.update).not.toHaveBeenCalled();
    expect(spies.createComment).not.toHaveBeenCalled();
  });
});

describe('handleGcpAlert — unknown severity', () => {
  test('files with severity:unknown but never adds claude-fix', async () => {
    const { tracker, spies } = makeTracker();
    await handleGcpAlert(
      baseIncident({
        condition: { displayName: 'A novel policy nobody has classified' },
        summary: 'something',
      }),
      tracker,
    );
    const labels = spies.create.mock.calls[0][0].labels as string[];
    expect(labels).toContain('severity:unknown');
    expect(labels).not.toContain(CLAUDE_FIX_LABEL);
  });
});

describe('parsePubsubMessage', () => {
  test('returns the parsed payload from message.json', () => {
    const incident = baseIncident();
    const msg = {
      data: '',
      messageId: 'm1',
      publishTime: '2026-05-07T00:00:00Z',
      json: incident,
    } as unknown as Parameters<typeof parsePubsubMessage>[0];
    expect(parsePubsubMessage(msg)?.incident.incident_id).toBe('inc-7771');
  });

  test('falls back to base64 decode when message.json is absent', () => {
    const incident = baseIncident();
    const data = Buffer.from(JSON.stringify(incident)).toString('base64');
    const msg = {
      data,
      messageId: 'm2',
      publishTime: '2026-05-07T00:00:00Z',
    } as unknown as Parameters<typeof parsePubsubMessage>[0];
    expect(parsePubsubMessage(msg)?.incident.incident_id).toBe('inc-7771');
  });

  test('returns null on unparseable JSON', () => {
    const msg = {
      data: Buffer.from('not json').toString('base64'),
      messageId: 'm3',
      publishTime: '2026-05-07T00:00:00Z',
    } as unknown as Parameters<typeof parsePubsubMessage>[0];
    expect(parsePubsubMessage(msg)).toBeNull();
  });

  test('returns null when payload lacks required fields', () => {
    const msg = {
      messageId: 'm4',
      publishTime: '2026-05-07T00:00:00Z',
      json: { version: '1.2', incident: { state: 'open' } /* missing incident_id */ },
      data: '',
    } as unknown as Parameters<typeof parsePubsubMessage>[0];
    expect(parsePubsubMessage(msg)).toBeNull();
  });
});
