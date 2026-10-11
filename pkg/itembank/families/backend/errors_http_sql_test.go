package backend

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_WrapIsChain(t *testing.T)     { itembanktest.CheckFamily(t, wrapIsChain{}) }
func TestFamily_PanicDeferTrace(t *testing.T) { itembanktest.CheckFamily(t, panicDeferTrace{}) }
func TestFamily_MiddlewareOrder(t *testing.T) { itembanktest.CheckFamily(t, middlewareOrder{}) }
func TestFamily_HandlerStatus(t *testing.T)   { itembanktest.CheckFamily(t, handlerStatus{}) }
func TestFamily_RankTies(t *testing.T)        { itembanktest.CheckFamily(t, rankTies{}) }
