# Security Policy

## Supported Versions

Security fixes are applied to:

- `main` (latest development branch)
- The latest tagged release, when tags exist

Older snapshots and untagged historical commits are not guaranteed to receive
backported fixes.

## Reporting a Vulnerability

Please do not report vulnerabilities in public issues.

Preferred process:

1. Use GitHub Security Advisories ("Report a vulnerability") if enabled for
   this repository.
2. If that is unavailable, contact the repository owner using the email listed
   on their GitHub profile and include "SECURITY" in the subject.

When reporting, include:

- Affected component(s) (Flutter app, Go engine, tool worker, scripts)
- Reproduction steps or proof of concept
- Impact assessment
- Suggested mitigations, if known

## Response Expectations

- Initial acknowledgement target: within 72 hours
- Triage and severity assessment target: within 7 days
- Fix timeline: depends on severity and exploitability

We will coordinate disclosure timing with reporters and request responsible
disclosure until a fix is available.

## Scope Notes

Particularly sensitive areas in this project include:

- Engine sandbox boundaries and file write restrictions
- Egress consent and model-provider request handling
- Key and credential handling paths
- Tool worker file parsing and office-format processing
