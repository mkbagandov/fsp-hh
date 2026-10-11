package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_KnnRegression(t *testing.T)     { itembanktest.CheckFamily(t, knnRegression{}) }
func TestFamily_LogregProbability(t *testing.T) { itembanktest.CheckFamily(t, logregProbability{}) }
func TestFamily_GiniBestSplit(t *testing.T)     { itembanktest.CheckFamily(t, giniBestSplit{}) }
func TestFamily_AbSampleSize(t *testing.T)      { itembanktest.CheckFamily(t, abSampleSize{}) }
