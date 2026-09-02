# Security

## Reporting

Report privately through
[GitHub Security Advisories](https://github.com/nickelsec/bough/security/advisories/new).
Please do not open a public issue for something unfixed.

Include the version or commit, and a transcript or steps that reproduce it if
you can. Expect a first reply within a week.

## What is worth reporting

bough parses files written by another program and serves a page on loopback,
so the interesting cases are:

- A crafted transcript that causes a crash, a hang, or a read outside the
  history directory
- Anything binding to an address other than `127.0.0.1`
- Any write under an agent's data directory. bough is strictly read only there
- Any outbound request from the served page. Everything it needs is embedded

Reading history that the user already owns is the whole point of the tool, not
a vulnerability.

There are no releases yet, so there is no supported version table. Fixes land
on `main`.
