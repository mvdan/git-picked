package main

// TODO: upstream this

import (
	"io"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

type commitChronoIterator struct {
	stack []*object.Commit
	seen  map[plumbing.Hash]bool
}

func NewCommitChronologicalIter(c *object.Commit, ignore []plumbing.Hash) object.CommitIter {
	seen := make(map[plumbing.Hash]bool)
	for _, h := range ignore {
		seen[h] = true
	}

	return &commitChronoIterator{
		stack: []*object.Commit{c},
		seen:  seen,
	}
}

func (w *commitChronoIterator) Next() (*object.Commit, error) {
	for {
		if len(w.stack) == 0 {
			return nil, io.EOF
		}

		sort.SliceStable(w.stack, func(i, j int) bool {
			t1 := w.stack[i].Committer.When
			t2 := w.stack[j].Committer.When
			return !t1.Before(t2)
		})

		c := w.stack[0]
		copy(w.stack, w.stack[1:])
		w.stack = w.stack[:len(w.stack)-1]

		if w.seen[c.Hash] {
			continue
		}

		w.seen[c.Hash] = true

		return c, c.Parents().ForEach(func(p *object.Commit) error {
			w.stack = append(w.stack, p)
			return nil
		})
	}
}

func (w *commitChronoIterator) ForEach(cb func(*object.Commit) error) error {
	for {
		c, err := w.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		err = cb(c)
		if err == storer.ErrStop {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (w *commitChronoIterator) Close() {}
