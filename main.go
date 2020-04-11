// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

func main() { os.Exit(main1()) }

func main1() int {
	branches, err := pickedBranches()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
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
	iter := NewCommitChronologicalIter(hcm, nil)
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

var buf bytes.Buffer

func commitKey(cm *object.Commit) string {
	buf.Reset()
	buf.WriteString(cm.Author.Name)
	buf.WriteString(cm.Author.Email)
	buf.WriteString(cm.Author.When.UTC().String())
	summary := cm.Message
	if i := strings.IndexByte(summary, '\n'); i > 0 {
		summary = summary[:i]
	}
	buf.WriteString(summary)
	return buf.String()
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
