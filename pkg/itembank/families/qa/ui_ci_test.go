package qa

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_LocatorCount(t *testing.T)    { itembanktest.CheckFamily(t, locatorCount{}) }
func TestFamily_AutowaitOutcome(t *testing.T) { itembanktest.CheckFamily(t, autowaitOutcome{}) }
func TestFamily_PytestSelection(t *testing.T) { itembanktest.CheckFamily(t, pytestSelection{}) }
func TestFamily_GitlabStagesRun(t *testing.T) { itembanktest.CheckFamily(t, gitlabStagesRun{}) }
