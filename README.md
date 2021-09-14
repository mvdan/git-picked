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
