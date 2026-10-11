package devops

import (
	"testing"

	"fsp/pkg/itembank/itembanktest"
)

func TestFamily_ChmodUmask(t *testing.T)     { itembanktest.CheckFamily(t, chmodUmask{}) }
func TestFamily_PipelineOutput(t *testing.T) { itembanktest.CheckFamily(t, pipelineOutput{}) }
func TestFamily_SignalExitCode(t *testing.T) { itembanktest.CheckFamily(t, signalExitCode{}) }
func TestFamily_CronNextRun(t *testing.T)    { itembanktest.CheckFamily(t, cronNextRun{}) }
