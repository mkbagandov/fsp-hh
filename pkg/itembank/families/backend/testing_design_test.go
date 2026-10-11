package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_BranchCoverage(t *testing.T)     { itembanktest.CheckFamily(t, branchCoverage{}) }
func TestFamily_CleanupOrder(t *testing.T)       { itembanktest.CheckFamily(t, cleanupOrder{}) }
func TestFamily_LittleLawCapacity(t *testing.T)  { itembanktest.CheckFamily(t, littleLawCapacity{}) }
func TestFamily_ReshardingMoves(t *testing.T)    { itembanktest.CheckFamily(t, reshardingMoves{}) }
func TestFamily_SelectDefaultDrops(t *testing.T) { itembanktest.CheckFamily(t, selectDefaultDrops{}) }
