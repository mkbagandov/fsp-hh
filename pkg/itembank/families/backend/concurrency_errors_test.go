package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ChannelFilterSum(t *testing.T) { itembanktest.CheckFamily(t, channelFilterSum{}) }
func TestFamily_DataRaceVars(t *testing.T)     { itembanktest.CheckFamily(t, dataRaceVars{}) }
func TestFamily_DeferNamedReturn(t *testing.T) { itembanktest.CheckFamily(t, deferNamedReturn{}) }
func TestFamily_LogLevelFilter(t *testing.T)   { itembanktest.CheckFamily(t, logLevelFilter{}) }
