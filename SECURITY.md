# Security Policy

## Reporting a Vulnerability

Please do not report vulnerabilities, tokens, API keys, JWTs, logs containing
authorization headers, or personal activity data in public issues.

Use GitHub private vulnerability reporting or a draft security advisory for
this repository. If private reporting is not available, open a minimal public
issue asking for a private contact path and omit exploit details, secrets,
personal data, and deployment-specific URLs.

Include enough non-sensitive context to reproduce the issue:

- affected version or commit;
- whether the deployment is authenticated or local unauthenticated mode;
- high-level impact;
- redacted request or response shapes when relevant.

## Data and Secret Handling

This project fronts Intervals.icu data for one athlete. Activity, wellness,
recovery, calendar, and training data can be sensitive personal health data.
Treat exported tool responses, logs, screenshots, and issue attachments as
private unless the athlete has explicitly chosen to publish them.

If a secret is exposed, rotate it before posting about the incident. This may
include the Intervals.icu API key, Supabase project keys, Supabase user
passwords, OAuth tokens, and any deployment platform secrets.

## Security Scope

The MCP server and CLI are intended to be read-only. The Intervals client should
only perform `GET` requests unless a future change explicitly documents,
reviews, and tests a writable feature.

Local mode disables Supabase/OIDC and should stay bound to loopback. Do not
expose local mode to a network you do not fully control.
