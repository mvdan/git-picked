// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"os"
	"strings"

	git "gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
)

const historyLimit = 500

func main() {
	branches, err := pickedBranches()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, b := range branches {
		fmt.Println(b)
	}
}

func pickedBranches() ([]string, error) {
	r, err := git.NewFilesystemRepository(".git")
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
	commitsLeft := make(map[string]*plumbing.Reference, len(all))
	picked := make([]string, 0)
	for _, ref := range all {
		if ref.Name() == head.Name() {
			continue
		}
		cm, err := r.Commit(ref.Hash())
		if err != nil {
			return nil, err
		}
		commitsLeft[commitStr(cm)] = ref
	}
	hcm, err := r.Commit(head.Hash())
	if err != nil {
		return nil, err
	}
	done := 0
	err = object.WalkCommitHistory(hcm, func(cm *object.Commit) error {
		if done++; done > historyLimit {
			return reachedEnd
		}
		str := commitStr(cm)
		ref, e := commitsLeft[str]
		if !e {
			// different commit
			return nil
		}
		delete(commitsLeft, str)
		picked = append(picked, ref.Name().Short())
		return nil
	})
	if err == reachedEnd {
		err = nil
	}
	return picked, err
}

var reachedEnd = fmt.Errorf("reached limit of %d commits", historyLimit)

func commitStr(cm *object.Commit) string {
	summary := cm.Message[:strings.IndexByte(cm.Message, '\n')]
	return fmt.Sprintf("%s %s %s %s",
		cm.Author.Name,
		cm.Author.Email,
		cm.Author.When.UTC().String(),
		summary,
	)
}

func allBranches(r *git.Repository) ([]*plumbing.Reference, error) {
	refs, err := r.References()
	if err != nil {
		return nil, err
	}
	defer refs.Close()
	all := make([]*plumbing.Reference, 0)
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.IsBranch() {
			all = append(all, ref)
		}
		return nil
	})
	return all, err
}
