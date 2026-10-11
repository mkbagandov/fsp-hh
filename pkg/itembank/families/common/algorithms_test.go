package common

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_RecursionComplexity(t *testing.T) { itembanktest.CheckFamily(t, recursionComplexity{}) }
func TestFamily_AmortizedResize(t *testing.T)     { itembanktest.CheckFamily(t, amortizedResize{}) }
func TestFamily_DequeOps(t *testing.T)            { itembanktest.CheckFamily(t, dequeOps{}) }
func TestFamily_HashCollisions(t *testing.T)      { itembanktest.CheckFamily(t, hashCollisions{}) }
func TestFamily_BSTTraversal(t *testing.T)        { itembanktest.CheckFamily(t, bstTraversal{}) }
func TestFamily_BFSDistance(t *testing.T)         { itembanktest.CheckFamily(t, bfsDistance{}) }
