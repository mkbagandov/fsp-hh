package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_SavepointBalance(t *testing.T) { itembanktest.CheckFamily(t, savepointBalance{}) }
func TestFamily_DeadlockPair(t *testing.T)     { itembanktest.CheckFamily(t, deadlockPair{}) }
func TestFamily_IsolationVisibleValue(t *testing.T) {
	itembanktest.CheckFamily(t, isolationVisibleValue{})
}
func TestFamily_NullAggregate(t *testing.T)     { itembanktest.CheckFamily(t, nullAggregate{}) }
func TestFamily_WindowTopPerGroup(t *testing.T) { itembanktest.CheckFamily(t, windowTopPerGroup{}) }
