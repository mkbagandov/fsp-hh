package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_TerraformApplyOrder(t *testing.T) { itembanktest.CheckFamily(t, terraformApplyOrder{}) }
func TestFamily_TerraformResourceCount(t *testing.T) {
	itembanktest.CheckFamily(t, terraformResourceCount{})
}
func TestFamily_CidrSplit(t *testing.T)     { itembanktest.CheckFamily(t, cidrSplit{}) }
func TestFamily_FirewallRules(t *testing.T) { itembanktest.CheckFamily(t, firewallRules{}) }
