package common

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_SQLCountNull(t *testing.T)    { itembanktest.CheckFamily(t, sqlCountNull{}) }
func TestFamily_SQLLeftJoinRows(t *testing.T) { itembanktest.CheckFamily(t, sqlLeftJoinRows{}) }
func TestFamily_SQLOrderLimit(t *testing.T)   { itembanktest.CheckFamily(t, sqlOrderLimit{}) }
func TestFamily_SQLSubqueryIn(t *testing.T)   { itembanktest.CheckFamily(t, sqlSubqueryIn{}) }
