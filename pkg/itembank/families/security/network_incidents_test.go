package security

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_CIDRBasics(t *testing.T)         { itembanktest.CheckFamily(t, cidrBasics{}) }
func TestFamily_FirewallFirstMatch(t *testing.T) { itembanktest.CheckFamily(t, firewallFirstMatch{}) }
func TestFamily_AffectedHosts(t *testing.T)      { itembanktest.CheckFamily(t, affectedHosts{}) }
func TestFamily_TimelineOrder(t *testing.T)      { itembanktest.CheckFamily(t, timelineOrder{}) }
