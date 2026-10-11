package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_MeanMedian(t *testing.T)      { itembanktest.CheckFamily(t, meanMedian{}) }
func TestFamily_PrecisionRecall(t *testing.T) { itembanktest.CheckFamily(t, precisionRecall{}) }
func TestFamily_DataLeakage(t *testing.T)     { itembanktest.CheckFamily(t, dataLeakage{}) }
