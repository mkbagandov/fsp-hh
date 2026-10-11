package qa

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ContractStatus(t *testing.T) { itembanktest.CheckFamily(t, contractStatus{}) }
func TestFamily_RetryPolicy(t *testing.T)    { itembanktest.CheckFamily(t, retryPolicy{}) }
func TestFamily_MaskOutput(t *testing.T)     { itembanktest.CheckFamily(t, maskOutput{}) }
func TestFamily_TeardownOrder(t *testing.T)  { itembanktest.CheckFamily(t, teardownOrder{}) }
