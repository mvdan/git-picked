// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

func main() { os.Exit(main1()) }

func main1() int {
	flag.Usage = func() { fmt.Fprintln(os.Stderr, `usage: git-picked [flags]`) }
	flag.Parse()
	if len(flag.Args()) > 0 {
		flag.Usage() // we don't take any args
	}

	branches, err := pickedBranches()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sort.Strings(branches)
	for _, b := range branches {
		fmt.Println(b)
	}
	return 0
}

type branchInfo struct {
	refs   []*plumbing.Reference
	author time.Time
}

func pickedBranches() ([]string, error) {
	openOpt := &git.PlainOpenOptions{DetectDotGit: true}
	r, err := git.PlainOpenWithOptions(".", openOpt)
	if err != nil {
		return nil, err
	}
	all, err := allBranches(r)
	if err != nil {
		return nil, err
	}
	head, err := r.Head()
	if err != nil {
		return nil, err
	}
	// commits not yet confirmed picked
	commitsLeft := make(map[string]branchInfo, len(all)-1)
	for _, ref := range all {
		// HEAD is obviously part of itself
		if ref.Name() == head.Name() {
			continue
		}
		cm, err := r.CommitObject(ref.Hash())
		if err != nil {
			return nil, err
		}
		key := commitKey(cm)
		prev := commitsLeft[key]
		commitsLeft[key] = branchInfo{
			refs:   append(prev.refs, ref),
			author: cm.Author.When.UTC(),
		}
	}
	if len(commitsLeft) == 0 {
		return nil, nil
	}
	hcm, err := r.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	stopTime := oldestTime(commitsLeft)
	picked := make([]string, 0)
	iter := object.NewCommitIterCTime(hcm, nil, nil)
	err = iter.ForEach(func(cm *object.Commit) error {
		if cm.Committer.When.Before(stopTime) {
			return storer.ErrStop
		}
		key := commitKey(cm)
		if bi, e := commitsLeft[key]; e {
			delete(commitsLeft, key)
			for _, ref := range bi.refs {
				picked = append(picked, ref.Name().Short())
			}
			if len(commitsLeft) == 0 {
				return storer.ErrStop
			}
			stopTime = oldestTime(commitsLeft)
		}
		return nil
	})
	return picked, err
}

func oldestTime(m map[string]branchInfo) (oldest time.Time) {
	first := true
	for _, bi := range m {
		if first || bi.author.Before(oldest) {
			oldest = bi.author
		}
		first = false
	}
	return
}

// commitKey returns a string that uniquely identifies a commit. If a commit
// message contains a Change-Id as described by
// https://gerrit-review.googlesource.com/Documentation/user-changeid.html, it
// will be returned directly. Otherwise, a string containing commit metadata
// will be returned instead, including the author information and the commit
// summary.
func commitKey(cm *object.Commit) string {
	const changeIdPrefix = "Change-Id: "
	// Split the lines. Trim spaces too, as commit messages often end in a
	// newline.
	lines := strings.Split(strings.TrimSpace(cm.Message), "\n")

	// Start from the bottom, as the Change-Id belongs in the footer.
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if line == "" {
			break // Change-Id can only be part of the footer
		}
		if !strings.HasPrefix(line, changeIdPrefix) {
			continue // not a Change-Id
		}
		// We found the Change-Id.
		id := strings.TrimSpace(line[len(changeIdPrefix):])
		if len(id) < 10 {
			// Gerrit's IDs are "I" + 40 hex chars.
			// Require at least 10, for minimum uniqueness.
			continue
		}
		return id
	}

	// No Change-Id found; fall back to inferring uniqueness from the
	// metadata.
	var b strings.Builder
	b.WriteString(cm.Author.Name)
	b.WriteString(cm.Author.Email)
	b.WriteString(cm.Author.When.UTC().String())
	summary := cm.Message
	if i := strings.IndexByte(summary, '\n'); i > 0 {
		summary = summary[:i]
	}
	b.WriteString(summary)
	return b.String()
}

func allBranches(r *git.Repository) ([]*plumbing.Reference, error) {
	refs, err := r.References()
	if err != nil {
		return nil, err
	}
	defer refs.Close()
	all := make([]*plumbing.Reference, 0)
	refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Name().IsBranch() {
			all = append(all, ref)
		}
		return nil
	})
	return all, nil
}
