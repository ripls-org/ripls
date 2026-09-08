# Security policy

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it through GitHub's private vulnerability reporting instead: go to the
**Security** tab of this repository and choose **Report a vulnerability**. That
opens a private advisory visible only to you and the maintainers, and it lets us
coordinate a fix and a disclosure timeline in the same place.

Useful things to include, to the extent you have them:

- What an attacker can do, and what they need in order to do it (an account? a
  community membership? nothing at all?).
- The affected component — Go server, Flutter client, deployment configuration.
- A reproduction: a request, a sequence of app actions, or a test case.
- The commit or release you observed it on.

We will acknowledge your report, tell you whether we can reproduce it, and keep
you updated as we work on a fix. If you would like credit in the advisory, say
so and tell us how you would like to be named.

## Scope

This repository holds the product: the server, the client, the protocol
definitions, and the reusable infrastructure modules. Vulnerabilities in that
code are in scope.

The deployment that runs at the project's own domains — its cloud configuration,
its secrets, its DNS — is operated separately and is not in this repository.
Report anything you find there through the same channel; it will reach the same
people.

## What is worth reporting

We are particularly interested in anything that lets someone:

- Read or modify data belonging to a community they are not a member of, or a
  user other than themselves.
- Bypass authentication, or escalate from a provisional identity to a full one
  without controlling the phone number or email involved.
- Extract another person's contact details, precise location, or private
  conversation content.
- Cause the server to make requests to arbitrary internal addresses, or to
  execute code.

This is a product about who knows whom in a neighborhood. Relationship and
location data is the sensitive asset here, more than any credential is — please
treat findings that expose it as high severity, and we will too.

## Out of scope

- Findings that require an attacker to already control the victim's device or
  account.
- Missing hardening headers, or automated-scanner output, with no demonstrated
  impact.
- Denial of service through sheer volume.
- Social engineering of maintainers or users.

## Supported versions

The project is pre-1.0 and moves quickly. Fixes land on `main` and ship in the
next release; there are no maintained back-support branches yet. If that changes,
this section will say so.
