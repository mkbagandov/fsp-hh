package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_FlexItemWidth(t *testing.T)     { itembanktest.CheckFamily(t, flexItemWidth{}) }
func TestFamily_BoxModelWidth(t *testing.T)     { itembanktest.CheckFamily(t, boxModelWidth{}) }
func TestFamily_GridAutoPlacement(t *testing.T) { itembanktest.CheckFamily(t, gridAutoPlacement{}) }
func TestFamily_MarginCollapse(t *testing.T)    { itembanktest.CheckFamily(t, marginCollapse{}) }
