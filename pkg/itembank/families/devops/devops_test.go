package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_DockerLayerCache(t *testing.T)  { itembanktest.CheckFamily(t, dockerLayerCache{}) }
func TestFamily_RollingUpdatePods(t *testing.T) { itembanktest.CheckFamily(t, rollingUpdatePods{}) }
