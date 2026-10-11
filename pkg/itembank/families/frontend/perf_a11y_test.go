package frontend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_MemoRenderCount(t *testing.T) { itembanktest.CheckFamily(t, memoRenderCount{}) }
func TestFamily_VirtualListRows(t *testing.T) { itembanktest.CheckFamily(t, virtualListRows{}) }
func TestFamily_AccessibleName(t *testing.T)  { itembanktest.CheckFamily(t, accessibleName{}) }
