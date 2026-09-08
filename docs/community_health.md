---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Why and how Ripls fosters tight, cohesive communities — the rationale for the 32-member size cap and the second-degree-connection ideal it protects.
  globs: [server/services/community/**]
  triggers: [community-size, member-limit, member-cap, cohesion, community-health]
  lens: [product, domain]
  skills: [issue, triage]
  domain: community
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Community Health

We seek to build and improve communities in the Ripls app. We need ways to measure and foster community health to assess whether we're achieving this goal.

## Community size: limited to 32 people

- The Ripls app is intended for tight, cohesive, high-participation communities.
- Ideal, every member is at most a second-degree connection with every other, meaning they know someone who knows every other community member.
- To help ensure this cohesiveness we choose to limit community size to 32.
- This number is partly inspired by the iMessage group size limit of 32: iMessage groups have proven they don't require explicit leadership or community moderation to function well.
