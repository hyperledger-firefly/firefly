# Hyperledger FireFly, an LF Decentralized Trust Project Security Policy

[LF Decentralized Trust Security Policy]: https://lf-decentralized-trust.github.io/governance/governing-documents/security

## About this document

This document defines how security vulnerability reporting is handled in Hyperledger FireFly, an LF Decentralized Trust Project. 
The approach aligns with the [LF Decentralized Trust Security Policy] . Please
review that document to understand the basis of the security reporting for Hyperledger FireFly.

This vulnerability policy borrows heavily from the
recommendations of the OpenSSF Vulnerability Disclosure working group. For
up-to-date information on the latest recommendations related to vulnerability
disclosures, please visit the [GitHub of that working
group](https://github.com/ossf/wg-vulnerability-disclosures).

If you are already familiar with the security policies of Hyperledger FireFly, and
ready to report a vulnerability, please jump to [Report Intakes](#report-intakes).

## Outline

This document has the following sections:

- [Hyperledger FireFly Security Policy](#project-an-lf-decentralized-trust-project-security-policy)
  - [Instructions](#instructions)
  - [About this document](#about-this-document)
  - [Outline](#outline)
  - [What Is a Vulnerability Disclosure Policy?](#what-is-a-vulnerability-disclosure-policy)
  - [Security Team](#security-team)
  - [Discussion Forums](#discussion-forums)
  - [Report Intakes](#report-intakes)
  - [CNA/CVE Reporting](#cnacve-reporting)
  - [Embargo List](#embargo-list)
  - [(GitHub) Security Advisories](#github-security-advisories)
  - [Private Patch Deployment Infrastructure](#private-patch-deployment-infrastructure)

## What Is a Vulnerability Disclosure Policy?

No piece of software is perfect. All software (at least, all software of a
certain size and complexity) has bugs. In open source development, members of
the community or the public find bugs and report them to the project. A
vulnerability disclosure policy explains how this process functions from the
perspective of the project.

This vulnerability disclosure policy explains the rules and guidelines for
Hyperledger FireFly. It is intended to act as both a reference for
outsiders–including both bug reporters and those looking for information on the
project’s security practices–as well as a set of rules that maintainers and
contributors have agreed to follow.

## Security Team

The current Hyperledger FireFly security team is:

| Name             | Email ID                            | Discord ID       | Area/Specialty    |
| ---------------- | ----------------------------------- | ---------------- | ----------------- |
| Enrique Lacal    | enrique.lacal@kaleido.io            | @enriquel8       | FireFly Core      | 
| Simon Gellis     | simon@sundae.fi                     | @SonicSwordcane  | Cardano connector | 
| Alexey Semenyuk  | aliaksei.semianiuk@instinctools.com | @alexey_semenyuk | Tezos connector   | 

The security team for Hyperledger FireFly must include at least three project
Maintainers that agree to carry out the following duties and responsibilities.
Members are added and removed from the team via approved Pull Requests to this
repository. For additional background into the role of the security team, see
the [People Infrastructure] section of the LF Decentralized Trust Security Policy.

[People Infrastructure]: https://lf-decentralized-trust.github.io/governance/governing-documents/security.html#people-infrastructure

**Responsibilities:**

1. Acknowledge the receipt of vulnerability reports to the reporter within 2
   business days.

2. Assess the issue. Engage with the reporter to ask any outstanding questions
about the report and how to reproduce it. If the report was received by email
and may be a security vulnerability, open a GitHub Security Advisory on the
repository to manage the report. If the report is not considered a
vulnerability, then the reporter should be informed and this process can be
halted. If the report is a regular bug (but not a security vulnerability), the
reporter should be informed (if necessary) of the regular process for reporting
issues.

3. Some issues may require more time and resources to correct. If a particular
report is complex, discuss an embargo period with the reporter during which
time the report will not be publicly disclosed. The embargo period should be
negotiated with the reporter and must not be longer than 90 days.

4. If necessary, create a private patch development infrastructure for the issue
   by emailing the [LF Decentralized Trust Community Architects].

[LF Decentralized Trust Community Architects]: mailto:community-architects@lfdecentralizedtrust.org

5. Request a CVE for the issue (see the [CNA/CVE Reporting](#cnacve-reporting)
   section).

6. Decide a date for the public release of the vulnerability report, the date
   the embargo period ends.

7. If applicable, notify members of the embargo list of the vulnerability,
upcoming patch and release, as described above.

8. Publish a new (software) release in which the vulnerability is addressed.

9. Publicly disclose the issue within 48 hours after the release via a
GitHub security advisory (see the [(GitHub) Security
Advisories](#github-security-advisories) section for details).

## Discussion Forums

Discussions about each reported vulnerability should be carried out in the
private GitHub security advisory about the vulnerability. If necessary, a private
channel specific to the issue may be created on the LF Decentralized Trust Discord server
with invited participants added to the discussion.

## Report Intakes

Hyperledger FireFly has the following ways to submit security
vulnerabilities. While the security team members will do their best to
respond to bugs disclosed in all possible ways, it is encouraged for bug
finders to report through the following approved channels:

- Email the [LF Decentralized Trust Foundation security
list](mailto:security@lists.lfdecentralizedtrust.org): To report a security issue, please
send an email with the name of the project/repository, a description of the issue, the
steps you took to create the issue, affected versions, and if known,
mitigations. If in triaging the email, the security team determines the issue may be
a security vulnerability, a [GitHub security vulnerability report] will be
opened.
- Open a [GitHub security vulnerability report]: Open a draft security advisory
on the "Security" tab of this GitHub repository. See [GitHub Security
Advisories](#github-security-advisories) to learn more about the security
infrastructure in GitHub.

[GitHub security vulnerability report]: https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability

## CNA/CVE Reporting

Hyperledger FireFly maintains a list of **Common Vulnerabilities and Exposures
(CVE)** and uses GitHub as its **CVE numbering authority (CNA)** for issuing
CVEs.

## Embargo List

Hyperledger FireFly does **NOT** currently maintain a private embargo list.

If you wish to be added to the embargo list, please email the [LF Decentralized Trust Foundation security
mailing list](mailto:security@lists.lfdecentralizedtrust.org), including the project name
(Hyperledger FireFly) and reason for being added to the embargo list. Requests
will be assessed by the Hyperledger FireFly security team in conjunction with the
appropriate LF Decentralized Trust Staff, and a decision will be made to accommodate or not
the request.

For more information about the embargo list, please see the [Embargo List
section of the LF Decentralized Trust Security
Policy](https://lf-decentralized-trust.github.io/governance/governing-documents/security.html#embargo-list).

## (GitHub) Security Advisories

Hyperledger FireFly uses GitHub Security Advisories to manage the public
disclosure of security vulnerabilities.

## Private Patch Deployment Infrastructure

In creating patches and new releases that address security vulnerabilities,
Hyperledger FireFly uses the private development features of GitHub for security
vulnerabilities. GitHub has [extensive
documentation](https://docs.github.com/en/code-security/security-advisories/repository-security-advisories)
about these features.
