package common

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_DFSOrder(t *testing.T)       { itembanktest.CheckFamily(t, dfsOrder{}) }
func TestFamily_HeapArray(t *testing.T)      { itembanktest.CheckFamily(t, heapArray{}) }
func TestFamily_LoopIterations(t *testing.T) { itembanktest.CheckFamily(t, loopIterations{}) }
func TestFamily_ForkCount(t *testing.T)      { itembanktest.CheckFamily(t, forkCount{}) }
func TestFamily_SQLLeftJoinAggregate(t *testing.T) {
	itembanktest.CheckFamily(t, sqlLeftJoinAggregate{})
}
