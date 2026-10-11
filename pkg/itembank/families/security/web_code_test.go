package security

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_UnsafeQueryFunctions(t *testing.T) {
	itembanktest.CheckFamily(t, unsafeQueryFunctions{})
}
func TestFamily_SameSiteCookie(t *testing.T)  { itembanktest.CheckFamily(t, sameSiteCookie{}) }
func TestFamily_VulnerableLine(t *testing.T)  { itembanktest.CheckFamily(t, vulnerableLine{}) }
func TestFamily_ValidatorOutput(t *testing.T) { itembanktest.CheckFamily(t, validatorOutput{}) }
