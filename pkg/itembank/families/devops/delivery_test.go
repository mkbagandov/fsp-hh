package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_StageOrder(t *testing.T)         { itembanktest.CheckFamily(t, stageOrder{}) }
func TestFamily_CacheKey(t *testing.T)           { itembanktest.CheckFamily(t, cacheKey{}) }
func TestFamily_CanaryTraffic(t *testing.T)      { itembanktest.CheckFamily(t, canaryTraffic{}) }
func TestFamily_TerraformPlan(t *testing.T)      { itembanktest.CheckFamily(t, terraformPlan{}) }
func TestFamily_AnsibleIdempotency(t *testing.T) { itembanktest.CheckFamily(t, ansibleIdempotency{}) }
func TestFamily_IncidentTriage(t *testing.T)     { itembanktest.CheckFamily(t, incidentTriage{}) }
