package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ClosureCounter(t *testing.T)  { itembanktest.CheckFamily(t, closureCounter{}) }
func TestFamily_NestedEventLoop(t *testing.T) { itembanktest.CheckFamily(t, nestedEventLoop{}) }
func TestFamily_TSConditionalDistribution(t *testing.T) {
	itembanktest.CheckFamily(t, tsConditionalDistribution{})
}
func TestFamily_TSUtilityAssignable(t *testing.T) { itembanktest.CheckFamily(t, tsUtilityAssignable{}) }
func TestFamily_BatchedRenderCount(t *testing.T)  { itembanktest.CheckFamily(t, batchedRenderCount{}) }
