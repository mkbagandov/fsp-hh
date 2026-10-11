package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_URLSearchParamsOutput(t *testing.T) {
	itembanktest.CheckFamily(t, urlSearchParamsOutput{})
}
func TestFamily_WebStorageOutput(t *testing.T)  { itembanktest.CheckFamily(t, webStorageOutput{}) }
func TestFamily_ForcedReflowCount(t *testing.T) { itembanktest.CheckFamily(t, forcedReflowCount{}) }
func TestFamily_UseMemoRecompute(t *testing.T)  { itembanktest.CheckFamily(t, useMemoRecompute{}) }
func TestFamily_LazyChunksBytes(t *testing.T)   { itembanktest.CheckFamily(t, lazyChunksBytes{}) }
