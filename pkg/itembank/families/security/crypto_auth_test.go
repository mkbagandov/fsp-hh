package security

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_CaesarXor(t *testing.T)     { itembanktest.CheckFamily(t, caesarXor{}) }
func TestFamily_ModexpDH(t *testing.T)      { itembanktest.CheckFamily(t, modexpDH{}) }
func TestFamily_TokenTTL(t *testing.T)      { itembanktest.CheckFamily(t, tokenTTL{}) }
func TestFamily_AuthFlowOrder(t *testing.T) { itembanktest.CheckFamily(t, authFlowOrder{}) }
