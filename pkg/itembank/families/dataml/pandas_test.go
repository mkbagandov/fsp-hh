package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_GroupbyAgg(t *testing.T)    { itembanktest.CheckFamily(t, groupbyAgg{}) }
func TestFamily_MergeRows(t *testing.T)     { itembanktest.CheckFamily(t, mergeRows{}) }
func TestFamily_LeftJoinCount(t *testing.T) { itembanktest.CheckFamily(t, leftJoinCount{}) }
func TestFamily_WindowRank(t *testing.T)    { itembanktest.CheckFamily(t, windowRank{}) }
