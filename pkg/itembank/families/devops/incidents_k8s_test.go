package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_IncidentTimeline(t *testing.T) { itembanktest.CheckFamily(t, incidentTimeline{}) }
func TestFamily_BurnRate(t *testing.T)         { itembanktest.CheckFamily(t, burnRate{}) }
func TestFamily_NodePodFit(t *testing.T)       { itembanktest.CheckFamily(t, nodePodFit{}) }
func TestFamily_CiJobConditions(t *testing.T)  { itembanktest.CheckFamily(t, ciJobConditions{}) }
