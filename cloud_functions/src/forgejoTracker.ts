// The private tracker on Forgejo, where every alert handler in the
// ripls-alerts codebase files its issues (#3293).
//
// Alerts are filed privately, not in the public code repository: a crash title
// can carry user data, a monitoring alert names infrastructure, and some
// alerts are security signals. Making the safe ones public is tracked
// separately.
//
// Identity: a dedicated Forgejo account, `ripls-alerts`, that can write issues
// in the tracker and nothing else. It is distinct from the account the Go
// server's feedback service uses (`ripls-feedback`), so each credential can be
// rotated or revoked alone. Its access token lives in GCP Secret Manager as
// `forgejo-alerts-token`.
//
// Forgejo differs from GitHub in two ways this file absorbs:
//   - an issue takes label ids, not names, so names are looked up and a
//     missing label is created (an alert must not be lost for want of one);
//   - a list is returned a page at a time with no promised order, so the
//     matches are collected and sorted here.

import { defineSecret } from 'firebase-functions/params';

import type {
  CreateIssueParams,
  IssueRef,
  IssueTracker,
  ListIssuesParams,
  RepoRef,
  TrackerIssue,
} from './issueTracker';

export const forgejoAlertsToken = defineSecret('forgejo-alerts-token');

// Where the tracker lives. Constants because the functions deploy against one
// tracker; promote them to defineString params if that ever stops being true.
export const TRACKER_URL = 'https://code.overt.org';
export const REPO_OWNER = 'ripls-org';
export const REPO_NAME = 'tracker';

/** Page size for list calls; Forgejo's default maximum. */
const PAGE_SIZE = 50;
/** Color for a label this code has to create; triage picks the real one. */
const NEW_LABEL_COLOR = '#ededed';
const REQUEST_TIMEOUT_MS = 15_000;

type Fetch = typeof fetch;

interface ForgejoLabel {
  id: number;
  name: string;
}

/**
 * Builds an IssueTracker for the Forgejo instance at `baseUrl`. `fetchImpl`
 * is injectable for tests.
 */
export function forgejoTracker(baseUrl: string, token: string, fetchImpl: Fetch = fetch): IssueTracker {
  const root = baseUrl.replace(/\/+$/, '');
  const repoPath = ({ owner, repo }: RepoRef, rest: string) =>
    `/api/v1/repos/${encodeURIComponent(owner)}/${encodeURIComponent(repo)}/${rest}`;

  async function call<T>(method: string, path: string, payload?: unknown): Promise<T> {
    const res = await fetchImpl(root + path, {
      method,
      headers: {
        Authorization: `token ${token}`,
        Accept: 'application/json',
        ...(payload === undefined ? {} : { 'Content-Type': 'application/json' }),
      },
      body: payload === undefined ? undefined : JSON.stringify(payload),
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
    if (!res.ok) {
      // Forgejo answers errors as {"message": "..."}; the token is never echoed.
      const text = await res.text().catch(() => '');
      let message = '';
      try {
        message = (JSON.parse(text) as { message?: string }).message ?? '';
      } catch {
        // Not JSON: the status alone is the error.
      }
      throw new Error(`Forgejo ${method} ${path}: ${res.status}${message ? ` ${message}` : ''}`);
    }
    return (res.status === 204 ? undefined : await res.json()) as T;
  }

  /** Every page of a list endpoint, up to `max` items. */
  async function listAll<T>(path: string, max = Infinity): Promise<T[]> {
    const out: T[] = [];
    const sep = path.includes('?') ? '&' : '?';
    for (let page = 1; out.length < max; page += 1) {
      const batch = await call<T[]>('GET', `${path}${sep}page=${page}&limit=${PAGE_SIZE}`);
      out.push(...batch);
      if (batch.length < PAGE_SIZE) break;
    }
    return out;
  }

  /** Label ids for the given names, creating any label that does not exist. */
  async function labelIds(repo: RepoRef, names: string[]): Promise<number[]> {
    if (names.length === 0) return [];
    const existing = new Map(
      (await listAll<ForgejoLabel>(repoPath(repo, 'labels'))).map((l) => [l.name, l.id]),
    );
    const ids: number[] = [];
    for (const name of names) {
      let id = existing.get(name);
      if (id === undefined) {
        const created = await call<ForgejoLabel>('POST', repoPath(repo, 'labels'), {
          name,
          color: NEW_LABEL_COLOR,
        });
        id = created.id;
        existing.set(name, id);
      }
      ids.push(id);
    }
    return ids;
  }

  return {
    issues: {
      async listForRepo(params: ListIssuesParams) {
        const query = new URLSearchParams({
          state: params.state,
          labels: params.labels,
          type: 'issues',
        });
        const listed = await listAll<TrackerIssue>(repoPath(params, `issues?${query}`));
        // Forgejo ignores a label filter that names a label the repository
        // does not have, and answers with every issue. A dedup label is new
        // on an incident's first firing, so without this check the first
        // alert of each incident would land on an unrelated issue.
        const found = listed.filter((issue) =>
          (issue.labels ?? []).some(
            (label) => (typeof label === 'string' ? label : label.name) === params.labels,
          ),
        );
        const byCreated = (a: TrackerIssue, b: TrackerIssue) =>
          Date.parse(a.created_at ?? '') - Date.parse(b.created_at ?? '');
        found.sort(params.direction === 'asc' ? byCreated : (a, b) => byCreated(b, a));
        return { data: found.slice(0, params.per_page) };
      },

      async create(params: CreateIssueParams) {
        const labels = await labelIds(params, params.labels);
        const created = await call<{ number: number }>('POST', repoPath(params, 'issues'), {
          title: params.title,
          body: params.body,
          labels,
        });
        return { data: { number: created.number } };
      },

      async createComment(params: IssueRef & { body: string }) {
        return call('POST', repoPath(params, `issues/${params.issue_number}/comments`), {
          body: params.body,
        });
      },

      async update(params: IssueRef & { state: 'open' | 'closed' }) {
        return call('PATCH', repoPath(params, `issues/${params.issue_number}`), {
          state: params.state,
        });
      },

      async addLabels(params: IssueRef & { labels: string[] }) {
        const labels = await labelIds(params, params.labels);
        return call('POST', repoPath(params, `issues/${params.issue_number}/labels`), { labels });
      },
    },
  };
}

/**
 * Builds the tracker from the bound secret. Trigger handlers call this once
 * per invocation and pass the result into upsertIssue.
 */
export function trackerFromSecret(): IssueTracker {
  const token = forgejoAlertsToken.value().trim();
  if (!token) {
    throw new Error('forgejo-alerts-token is empty');
  }
  return forgejoTracker(TRACKER_URL, token);
}
