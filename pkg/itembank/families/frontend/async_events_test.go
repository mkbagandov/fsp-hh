package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_PromiseChainOutput(t *testing.T) { itembanktest.CheckFamily(t, promiseChainOutput{}) }
func TestFamily_AsyncAwaitOrder(t *testing.T)    { itembanktest.CheckFamily(t, asyncAwaitOrder{}) }
func TestFamily_EventPropagation(t *testing.T)   { itembanktest.CheckFamily(t, eventPropagation{}) }
func TestFamily_DebounceThrottleCount(t *testing.T) {
	itembanktest.CheckFamily(t, debounceThrottleCount{})
}
