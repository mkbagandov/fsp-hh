package common

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_LoopComplexity(t *testing.T)    { itembanktest.CheckFamily(t, loopComplexity{}) }
func TestFamily_GrowthOrder(t *testing.T)       { itembanktest.CheckFamily(t, growthOrder{}) }
func TestFamily_StackOps(t *testing.T)          { itembanktest.CheckFamily(t, stackOps{}) }
func TestFamily_BinarySearchSteps(t *testing.T) { itembanktest.CheckFamily(t, binarySearchSteps{}) }
func TestFamily_SubnetHosts(t *testing.T)       { itembanktest.CheckFamily(t, subnetHosts{}) }
