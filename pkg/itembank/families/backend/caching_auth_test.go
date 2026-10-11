package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_LRUMisses(t *testing.T)      { itembanktest.CheckFamily(t, lruMisses{}) }
func TestFamily_TTLOriginHits(t *testing.T)  { itembanktest.CheckFamily(t, ttlOriginHits{}) }
func TestFamily_JWTClaimsCheck(t *testing.T) { itembanktest.CheckFamily(t, jwtClaimsCheck{}) }
func TestFamily_CORSOutcome(t *testing.T)    { itembanktest.CheckFamily(t, corsOutcome{}) }
