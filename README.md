# git-picked

This tool is a drop-in replacement for `git branch --merged` which also
works when branches are rebased or cherry-picked into `HEAD`.

	go get -u github.com/mvdan/git-picked

It matches commits via a hash containing:

* Author name
* Author email
* Author date (in UTC)
* Commit summary (first line of its message)

Note that the matching is only done with the tip commit of each branch
for simplicity. Thus this tool will be wrong if only some of the commits
of a branch ahead of HEAD are cherry-picked, but not the tip one. This
might change in the future.

It is a standalone binary and does not depend on the `git` executable.
