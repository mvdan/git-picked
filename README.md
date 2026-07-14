# git-picked

This tool is a drop-in replacement for `git branch --merged` which also works
when branches are rebased or cherry-picked into `HEAD`.

	go install mvdan.cc/git-picked@latest

It tries to match commits via their
[Change-Id](https://gerrit-review.googlesource.com/Documentation/user-changeid.html),
if it is present. Otherwise, a hash is used consisting of:

* Author name
* Author email
* Author date (in UTC)
* Commit summary (first line of its message)

Note that the matching is only done with the tip commit of each branch.

Matching is done against the history of `HEAD`, stopping when either all commits
have been found or when the main history dates fall behind the author dates of
the commits left to match. This will work nicely as long as noone uses a time
machine.

This is a standalone binary and does not depend on the `git` executable.

Note that this heuristic may get confused with release branches. As such, if you
name your release branches `release-x.y` you likely want to use an alias like:

	git-picked | grep -vE '^(master|release|backport)'

Branches with patches targeting branches other than master should also be
excluded, like `backport-some-feature` in this case.

## Unpushed branches

	git-picked -unpushed [-remote origin]

This mode lists the branches which contain commits not yet pushed to Gerrit,
with a tab-separated count of such commits per branch, sorted by the commit
time of each branch's tip, newest first.

Each branch is checked against the remote it tracks, falling back to the
`-remote` flag for branches which do not track one; each remote is listed
at most once.

Every patchset pushed for review is advertised by the remote as a `refs/changes/*` ref,
so a commit which is not on any remote branch and whose hash is not one of
those refs has never been pushed in its current form. Note that this matches
exact commits: amending a pushed commit counts as unpushed again.

Any credentials which git has configured for the remote's URL are used to
list the remote, looked up by running `git credential fill` so that git's
credential helpers are supported. When git has none, or when the git binary
is unavailable, the remote is listed without credentials.
