// The issue tracker as the alert handlers see it: the five operations they
// use, in the shape they have always called them (one options object in,
// `{ data }` out). Handlers and upsertIssue depend on this interface only, so
// tests pass a fake and the forge behind it lives in one file
// (forgejoTracker.ts).

/** The fields of an issue the handlers read. */
export interface TrackerIssue {
  number: number;
  state: string;
  /** Present when the item is a pull request; such items are ignored. */
  pull_request?: unknown;
  labels?: Array<string | { name?: string }>;
  created_at?: string;
}

export interface RepoRef {
  owner: string;
  repo: string;
}

export interface ListIssuesParams extends RepoRef {
  state: 'open' | 'closed' | 'all';
  /** One label name; only issues carrying it are returned. */
  labels: string;
  /** Upper bound on the number of issues returned. */
  per_page: number;
  sort: 'created';
  direction: 'asc' | 'desc';
}

export interface CreateIssueParams extends RepoRef {
  title: string;
  body: string;
  /** Label names. A tracker that needs ids resolves them itself. */
  labels: string[];
}

export interface IssueRef extends RepoRef {
  issue_number: number;
}

export interface IssueTracker {
  issues: {
    listForRepo(params: ListIssuesParams): Promise<{ data: TrackerIssue[] }>;
    create(params: CreateIssueParams): Promise<{ data: { number: number } }>;
    createComment(params: IssueRef & { body: string }): Promise<unknown>;
    update(params: IssueRef & { state: 'open' | 'closed' }): Promise<unknown>;
    addLabels(params: IssueRef & { labels: string[] }): Promise<unknown>;
  };
}
