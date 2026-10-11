package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_AvailabilityComposition(t *testing.T) {
	itembanktest.CheckFamily(t, availabilityComposition{})
}
func TestFamily_FailingCases(t *testing.T) { itembanktest.CheckFamily(t, failingCases{}) }
