// Copyright (c) 2019, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	os.Exit(testscript.RunMain(m, map[string]func() int{
		"git-picked": main1,
		"join-lines": joinLines,
	}))
}

// joinLines is a little helper, since it's impossible to have multiline strings
// in testscript files.
func joinLines() int {
	for _, arg := range os.Args[1:] {
		fmt.Println(arg)
	}
	return 0
}

func TestScript(t *testing.T) {
	t.Parallel()
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join("testdata", "script"),
		RequireExplicitExec: true,
	})
}
