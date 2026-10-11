package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_PortMapping(t *testing.T)   { itembanktest.CheckFamily(t, portMapping{}) }
func TestFamily_EntrypointCmd(t *testing.T) { itembanktest.CheckFamily(t, entrypointCmd{}) }
func TestFamily_QosClass(t *testing.T)      { itembanktest.CheckFamily(t, qosClass{}) }
func TestFamily_HpaReplicas(t *testing.T)   { itembanktest.CheckFamily(t, hpaReplicas{}) }
func TestFamily_ProbeRestart(t *testing.T)  { itembanktest.CheckFamily(t, probeRestart{}) }
