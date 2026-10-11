package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_StateBatchingCount(t *testing.T) { itembanktest.CheckFamily(t, stateBatchingCount{}) }
func TestFamily_EffectOrder(t *testing.T)        { itembanktest.CheckFamily(t, effectOrder{}) }
func TestFamily_ListKeysInput(t *testing.T)      { itembanktest.CheckFamily(t, listKeysInput{}) }
func TestFamily_TSNarrowing(t *testing.T)        { itembanktest.CheckFamily(t, tsNarrowing{}) }
