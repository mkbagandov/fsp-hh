package dataml

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ListSlicing(t *testing.T)      { itembanktest.CheckFamily(t, listSlicing{}) }
func TestFamily_GeneratorExhaust(t *testing.T) { itembanktest.CheckFamily(t, generatorExhaust{}) }
func TestFamily_MutableDefault(t *testing.T)   { itembanktest.CheckFamily(t, mutableDefault{}) }
