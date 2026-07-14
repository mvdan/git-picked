// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

var (
	unpushedFlag = flag.Bool("unpushed", false, "list branches with commits not yet pushed to Gerrit")
	remoteFlag   = flag.String("remote", "origin", "the -unpushed remote for branches which do not track one")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, `usage: git-picked [flags]`)
		flag.PrintDefaults()
	}
	flag.Parse()
	if len(flag.Args()) > 0 {
		flag.Usage() // we don't take any args
	}

	if *unpushedFlag {
		branches, err := unpushedBranches(*remoteFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, b := range branches {
			fmt.Printf("%s\t%d\n", b.name, b.count)
		}
		return
	}

	branches, err := pickedBranches()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sort.Strings(branches)
	for _, b := range branches {
		fmt.Println(b)
	}
}

type branchInfo struct {
	refs   []*plumbing.Reference
	author time.Time
}

func openRepository() (*git.Repository, error) {
	return git.PlainOpenWithOptions(".", &git.PlainOpenOptions{
		DetectDotGit: true,
		// Follow a linked worktree's commondir to the shared refs and
		// objects; without this, no branches resolve from a worktree.
		// Harmless for regular repositories.
		EnableDotGitCommonDir: true,
	})
}

func pickedBranches() ([]string, error) {
	r, err := openRepository()
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

type unpushedBranch struct {
	name  string
	count int
	tip   time.Time
}

// unpushedBranches lists the local branches which contain commits not yet
// pushed to Gerrit, with a count of such commits per branch. Every pushed
// patchset is advertised by a Gerrit remote as a refs/changes/* ref, so a
// commit which is not on any remote branch and whose hash is not one of
// those refs has never been pushed in its current form. Each branch is
// checked against the remote it tracks, falling back to defaultRemote, and
// each remote is listed only once. The branches are sorted by the commit
// time of their tip commits, newest first.
func unpushedBranches(defaultRemote string) ([]unpushedBranch, error) {
	r, err := openRepository()
	if err != nil {
		return nil, err
	}
	cfg, err := r.Config()
	if err != nil {
		return nil, err
	}
	all, err := allBranches(r)
	if err != nil {
		return nil, err
	}
	byRemote := make(map[string][]*plumbing.Reference)
	for _, ref := range all {
		remoteName := defaultRemote
		if bc := cfg.Branches[ref.Name().Short()]; bc != nil &&
			bc.Remote != "" && bc.Remote != "." {
			remoteName = bc.Remote
		}
		byRemote[remoteName] = append(byRemote[remoteName], ref)
	}
	var result []unpushedBranch
	for remoteName, branchRefs := range byRemote {
		unpushed, err := unpushedForRemote(r, remoteName, branchRefs)
		if err != nil {
			return nil, err
		}
		result = append(result, unpushed...)
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].tip.Equal(result[j].tip) {
			return result[i].tip.After(result[j].tip)
		}
		return result[i].name < result[j].name
	})
	return result, nil
}

// unpushedForRemote implements unpushedBranches for the branches tracking
// one remote, listing it just once.
func unpushedForRemote(r *git.Repository, remoteName string, branchRefs []*plumbing.Reference) ([]unpushedBranch, error) {
	remote, err := r.Remote(remoteName)
	if err != nil {
		return nil, err
	}
	remoteRefs, err := remote.List(&git.ListOptions{
		Auth: gitCredentials(remote.Config().URLs[0]),
	})
	if err != nil {
		return nil, err
	}
	pushed := make(map[plumbing.Hash]bool)
	for _, ref := range remoteRefs {
		if strings.HasPrefix(ref.Name().String(), "refs/changes/") {
			pushed[ref.Hash()] = true
		}
	}

	// Collect all commits reachable from the remote-tracking branches;
	// they are on the remote already, and so not of interest.
	onRemote := make(map[plumbing.Hash]bool)
	refs, err := r.References()
	if err != nil {
		return nil, err
	}
	defer refs.Close()
	prefix := "refs/remotes/" + remoteName + "/"
	var stack []plumbing.Hash
	refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() == plumbing.HashReference &&
			strings.HasPrefix(ref.Name().String(), prefix) {
			stack = append(stack, ref.Hash())
		}
		return nil
	})
	for len(stack) > 0 {
		h := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if onRemote[h] {
			continue
		}
		onRemote[h] = true
		cm, err := r.CommitObject(h)
		if err != nil {
			return nil, err
		}
		stack = append(stack, cm.ParentHashes...)
	}

	var result []unpushedBranch
	for _, ref := range branchRefs {
		count := 0
		seen := make(map[plumbing.Hash]bool)
		stack := []plumbing.Hash{ref.Hash()}
		for len(stack) > 0 {
			h := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[h] || onRemote[h] {
				continue
			}
			seen[h] = true
			if !pushed[h] {
				count++
			}
			cm, err := r.CommitObject(h)
			if err != nil {
				return nil, err
			}
			stack = append(stack, cm.ParentHashes...)
		}
		if count > 0 {
			cm, err := r.CommitObject(ref.Hash())
			if err != nil {
				return nil, err
			}
			result = append(result, unpushedBranch{
				name:  ref.Name().Short(),
				count: count,
				tip:   cm.Committer.When,
			})
		}
	}
	return result, nil
}

// gitCredentials returns the credentials which git has configured for the
// URL, looked up by running "git credential fill" so that git's credential
// helpers are supported. It returns nil when there are none, such as when no
// helper knows about the host or the git binary is unavailable.
func gitCredentials(rawURL string) transport.AuthMethod {
	if !strings.HasPrefix(rawURL, "https://") && !strings.HasPrefix(rawURL, "http://") {
		return nil
	}
	cmd := exec.Command("git", "credential", "fill")
	// Only use stored credentials; never prompt interactively.
	cmd.Env = append(cmd.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = strings.NewReader("url=" + rawURL + "\n\n")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	var auth githttp.BasicAuth
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSuffix(line, "\n")
		if v, ok := strings.CutPrefix(line, "username="); ok {
			auth.Username = v
		} else if v, ok := strings.CutPrefix(line, "password="); ok {
			auth.Password = v
		}
	}
	if auth == (githttp.BasicAuth{}) {
		return nil
	}
	return &auth
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
