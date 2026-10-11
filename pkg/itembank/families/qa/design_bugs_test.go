package qa

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_BoundaryValues(t *testing.T) { itembanktest.CheckFamily(t, boundaryValues{}) }
func TestFamily_PairwiseGaps(t *testing.T)   { itembanktest.CheckFamily(t, pairwiseGaps{}) }
func TestFamily_TriageSeverity(t *testing.T) { itembanktest.CheckFamily(t, triageSeverity{}) }
func TestFamily_BisectCulprit(t *testing.T)  { itembanktest.CheckFamily(t, bisectCulprit{}) }
