package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_OneHotShape(t *testing.T)     { itembanktest.CheckFamily(t, oneHotShape{}) }
func TestFamily_ScalerTransform(t *testing.T) { itembanktest.CheckFamily(t, scalerTransform{}) }
func TestFamily_TargetEncoding(t *testing.T)  { itembanktest.CheckFamily(t, targetEncoding{}) }
func TestFamily_PivotTable(t *testing.T)      { itembanktest.CheckFamily(t, pivotTable{}) }
