package common

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_HTTPCacheHeaders(t *testing.T) { itembanktest.CheckFamily(t, httpCacheHeaders{}) }
func TestFamily_HTTPIdempotentMethods(t *testing.T) {
	itembanktest.CheckFamily(t, httpIdempotentMethods{})
}
func TestFamily_HTTPRedirectCodes(t *testing.T) { itembanktest.CheckFamily(t, httpRedirectCodes{}) }
func TestFamily_OSSchedulingOrder(t *testing.T) { itembanktest.CheckFamily(t, osSchedulingOrder{}) }
func TestFamily_OSFileDescriptors(t *testing.T) { itembanktest.CheckFamily(t, osFileDescriptors{}) }
func TestFamily_NetTCPvsUDP(t *testing.T)       { itembanktest.CheckFamily(t, netTCPvsUDP{}) }
