---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Testing strategy across client and server — unit/integration/e2e layers, testcontainers, ViewModel-first client tests, mock patterns.
  globs: [server/**/*_test.go, app/test/**, e2e/**]
  triggers: [test, testing, mock, testcontainer, coverage, integration]
  lens: [testing]
  alwaysApply: true
  domain: testing
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Testing the Ripls client and server

## Testing philosophy

Comprehensive testing is a big priority for the Ripls server and clients, and their integration. Testing is about development velocity: when all modules and the interaction between them is well-covered by tests, we can confidently make changes, additions, and subtractions from the code base and no there are no regressions. Tests also define the contracts of modules; good tests make it easy to assemble modules together and make it fast and reliable to call on help from LLMs to expand and maintain our codebase.

## Layers of testing (applies to client and server)

Testing occurs at several levels, starting with small modules and moving up the entire client or server, and also the interactions of client and server through entire workflows:

### Unit tests

- Tests individual functions and libraries with as little interaction between modules as possible.
- No dependencies on outside services; the focus is on speed while verifying algorithmic correctness.
- The strength of unit tests is to cover virtually all code with basic assurance that it does what it says in its contract and in isolation.
- Where interactions can't be avoided, mocks or fakes are used to stub out dependencies.
- Examples of these tests are alongside services (server/services/*/*test.go) or helper libraries, or on the client tests for ViewModels or Widgets.

### Module integration tests

- Tests significant chunks of functionality, like the chat system or generative AI system.
- Where feasible, actually connects to real dependencies (e.g. LLM providers) to verify the connections work.
- These tests can be slower than unit tests but they should still finish in no more than a minute.
- Examples of these tests are the AI provider test (server/ai/provider_integration_test.go) and the per-feature server integration tests (server/integration_tests/).

### System integration tests / end-to-end tests / workflow tests

- Tests the interaction of most or all of the client/server ecosystem, including entire user journeys involving multiple client instances and a real server backend.
- For example, we might test the entire workflow of offering gear to giveaway, multiple users expressing interest in it, the owner selecting a recipient, and the archiving of the giveaway.
- Today these are covered server-side by Go end-to-end tests (`server/e2e_test.go`), the per-feature integration suite (`server/integration_tests/`), and the `server/simulation/` scenario harness. Client-side UI automation runs as a *selective* Playwright suite (`e2e/`) that drives the production Flutter Web bundle in a real browser, complementing — not replacing — the server tests (the earlier Patrol approach was retired in #1887). See [update_system_tests.md](update_system_tests.md) and [`e2e/README.md`](../../e2e/README.md).
