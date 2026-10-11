package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ConfidenceInterval(t *testing.T) { itembanktest.CheckFamily(t, confidenceInterval{}) }
func TestFamily_BinomialPValue(t *testing.T)     { itembanktest.CheckFamily(t, binomialPValue{}) }
func TestFamily_PipelineOrder(t *testing.T)      { itembanktest.CheckFamily(t, pipelineOrder{}) }
func TestFamily_MultipleTesting(t *testing.T)    { itembanktest.CheckFamily(t, multipleTesting{}) }
