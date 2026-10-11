package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ErrorBudget(t *testing.T)       { itembanktest.CheckFamily(t, errorBudget{}) }
func TestFamily_AlertThreshold(t *testing.T)    { itembanktest.CheckFamily(t, alertThreshold{}) }
func TestFamily_PromqlCounterRate(t *testing.T) { itembanktest.CheckFamily(t, promqlCounterRate{}) }
func TestFamily_NatTable(t *testing.T)          { itembanktest.CheckFamily(t, natTable{}) }
func TestFamily_DNSTTL(t *testing.T)            { itembanktest.CheckFamily(t, dnsTTL{}) }
