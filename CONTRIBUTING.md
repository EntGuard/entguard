# Contributing to EntGuard

Thanks for your interest in EntGuard. This document describes how to report
issues, how to submit changes, and the licensing terms every contribution must
meet.

## License

EntGuard is distributed under the [Apache License 2.0](LICENSE). Every
contribution must be compliant with this license. By submitting a contribution
you agree that it is licensed under the same terms.

New source files should carry an SPDX identifier in the header:

```
// SPDX-License-Identifier: Apache-2.0
// Copyright (c) <year> <your name or organization>
```

Do not add or modify license headers in files you did not author.

## Developer Certificate of Origin

To indicate your acceptance of the Developer Certificate of Origin 1.1 and the
licensing of your contribution under the Apache License 2.0, add a `Signed-off-by`
line to the end of every commit message:

```
Signed-off-by: Your Name <your@email.example.org>
```

Git adds this line for you when you commit with `-s`:

```sh
git commit -s
```

Configure your identity once so the sign-off is generated correctly:

```sh
git config --global user.name "Your Name"
git config --global user.email "your@email.example.org"
```

Requirements for the sign-off:

* Use your real name. Pseudonyms and anonymous contributions are not accepted.
* The email address must match the one in the commit author field.
* Every commit in a pull request must be signed off, not just the first one.

If you forgot the sign-off, amend the last commit:

```sh
git commit --amend -s --no-edit
```

For multiple commits, rewrite the whole branch:

```sh
git rebase --signoff upstream/main
```

Then force-push the branch. A DCO check runs on every pull request and fails if
any commit is missing a valid sign-off.

## Reporting security vulnerabilities

Do not open a public issue for a suspected security vulnerability. Follow the
reporting process described in [SECURITY.md](SECURITY.md).

## Reporting bugs

Bugs and feature requests are tracked in
[GitHub Issues](https://github.com/entguard/entguard/issues). Open an issue and
include:

* EntGuard version or commit hash
* Platform and runtime environment
* Exact steps to reproduce
* Expected and observed behavior
* Relevant logs, stack traces, or configuration, with secrets removed

Search existing issues first. If you hit a known problem, add your reproduction
details to the existing issue rather than opening a duplicate.

## Proposing changes

For bug fixes, small refactors, documentation, and tests, open a pull request
directly.

For anything that changes public APIs, data models, wire formats, configuration
schemas, or dependencies, open an issue first and agree on the approach before
writing the code. This avoids rework on both sides.

## Development workflow

EntGuard uses the fork and pull request model. Contributors do not push branches
to the upstream repository. Fork `entguard/entguard` on GitHub, work in your
fork, and open a pull request against upstream `main`.

```sh
git clone https://github.com/<your-user>/entguard.git
cd entguard
git remote add upstream https://github.com/entguard/entguard.git
git fetch upstream
git checkout -b <topic-branch> upstream/main
```

Keep your fork current by rebasing on `upstream/main`, not by merging:

```sh
git fetch upstream
git rebase upstream/main
git push --force-with-lease origin <topic-branch>
```

Then:

1. Keep each commit a single logical change. Separate refactoring commits from
   behavioral changes.
3. Add or update tests to cover the change. Bug fixes should include a test that
   fails before the fix.
4. Update the documentation affected by the change in the same pull request.
5. Rebase onto current `upstream/main` before opening the pull request and
   whenever the branch falls behind.
6. Verify locally before pushing

## Commit messages

```
component: short imperative summary under 72 characters

Explain what the change does and why it is needed. Describe the previous
behavior and the new behavior. Wrap the body at 72 columns.

Fixes: #123
Signed-off-by: Your Name <your@email.example.org>
```

Reference the issue the commit addresses with `Fixes: #<id>` or `Refs: #<id>`.

## Pull requests

* Keep pull requests focused. Unrelated changes belong in separate pull requests.
* Mark work in progress as a draft.
* CI must pass. Pull requests with failing checks are not reviewed.
* Address review comments with additional commits during review, then squash or
  clean up the history before the branch is merged.
* At least one maintainer approval is required to merge.

## Questions

Open a [GitHub issue](https://github.com/entguard/entguard/issues) for design
questions and usage problems. Keep the discussion in the issue tracker so the
answer stays searchable for other contributors.

---

## Developer Certificate of Origin 1.1

Source: [https://developercertificate.org/](https://developercertificate.org/)

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.


Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```
