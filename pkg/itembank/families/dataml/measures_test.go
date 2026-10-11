package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_VarianceStd(t *testing.T)        { itembanktest.CheckFamily(t, varianceStd{}) }
func TestFamily_PearsonCorrelation(t *testing.T) { itembanktest.CheckFamily(t, pearsonCorrelation{}) }
func TestFamily_ZTestConclusion(t *testing.T)    { itembanktest.CheckFamily(t, zTestConclusion{}) }
func TestFamily_MaeRmse(t *testing.T)            { itembanktest.CheckFamily(t, maeRmse{}) }
func TestFamily_RocAucPairs(t *testing.T)        { itembanktest.CheckFamily(t, rocAucPairs{}) }
func TestFamily_CVScheme(t *testing.T)           { itembanktest.CheckFamily(t, cvScheme{}) }
