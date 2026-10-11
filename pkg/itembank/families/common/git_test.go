package common

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_GitBranchCommits(t *testing.T) { itembanktest.CheckFamily(t, gitBranchCommits{}) }
func TestFamily_GitRebaseMerge(t *testing.T)   { itembanktest.CheckFamily(t, gitRebaseMerge{}) }
func TestFamily_GitResetModes(t *testing.T)    { itembanktest.CheckFamily(t, gitResetModes{}) }
func TestFamily_GitConflictSteps(t *testing.T) { itembanktest.CheckFamily(t, gitConflictSteps{}) }
