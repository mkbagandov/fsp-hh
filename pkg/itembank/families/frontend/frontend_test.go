package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ClosureLoop(t *testing.T)    { itembanktest.CheckFamily(t, closureLoop{}) }
func TestFamily_EventLoopOrder(t *testing.T) { itembanktest.CheckFamily(t, eventLoopOrder{}) }
func TestFamily_CSSSpecificity(t *testing.T) { itembanktest.CheckFamily(t, cssSpecificity{}) }
