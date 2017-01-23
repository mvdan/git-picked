# git-picked

This tool is a drop-in replacement for `git branch --merged` which also
works when branches are rebased or cherry-picked into `HEAD`.

	go get -u github.com/mvdan/git-picked

It matches commits by:

* Author name
* Author email
* Author date (in UTC)
* Commit summary (first line of its message)

It is a standalone binary and does not depend on the `git` executable.
