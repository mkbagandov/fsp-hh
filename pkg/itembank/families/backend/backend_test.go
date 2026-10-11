package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_SQLJoinCount(t *testing.T)     { itembanktest.CheckFamily(t, sqlJoinCount{}) }
func TestFamily_SQLGroupHaving(t *testing.T)   { itembanktest.CheckFamily(t, sqlGroupHaving{}) }
func TestFamily_HTTPStatusCodes(t *testing.T)  { itembanktest.CheckFamily(t, httpStatusCodes{}) }
func TestFamily_IsolationAnomaly(t *testing.T) { itembanktest.CheckFamily(t, isolationAnomaly{}) }
func TestFamily_GoroutineCounter(t *testing.T) { itembanktest.CheckFamily(t, goroutineCounter{}) }
