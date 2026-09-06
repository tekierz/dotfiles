# Security Policy

## Supported Versions

Security fixes are applied to the latest stable release and the default branch.
Older releases may not receive patches. If no stable release has been published
yet, only the default branch is supported.

## Reporting a Vulnerability

Do not open a public issue for a suspected vulnerability.

Use GitHub's private vulnerability reporting flow:

<https://github.com/tekierz/dotfiles/security/advisories/new>

Include:

- the affected version or commit;
- the operating system and architecture;
- the shortest safe reproduction you can provide;
- the expected and observed security boundary;
- the potential impact; and
- any suggested mitigation.

Do not include real credentials, private configuration, raw operation journals,
or unrelated personal data. Prefer a minimal temporary environment. The
maintainer will acknowledge a report when practical, investigate it, and
coordinate disclosure after a fix is available. No response-time or bounty
commitment is currently offered.

If GitHub private vulnerability reporting is unavailable, privately contact the
maintainer using a contact method published on the
[@tekierz GitHub profile](https://github.com/tekierz). Do not fall back to a
public issue.

## Security Scope

Reports about the following project-owned behavior are in scope:

- command execution and privilege boundaries;
- plan/apply authority validation;
- configuration, backup, restore, and uninstall path safety;
- sensitive-data exposure in reviewed public output;
- release artifact integrity; and
- dependency vulnerabilities reachable from the application.

Vulnerabilities in an upstream package manager, tool, theme, MCP server, npm
package, or downloaded configuration should normally be reported to that
upstream project. Reports showing that `dotfiles` invokes or configures an
upstream component unsafely remain in scope here.

## Safe Research

Test only systems and data you own or are authorized to use. Do not perform
denial-of-service testing, access another person's data, or publish an
uncoordinated exploit. Stop testing and report the issue if you encounter
sensitive information.
