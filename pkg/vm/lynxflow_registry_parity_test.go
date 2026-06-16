package vm

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

func TestLynxFlowScalarRegistryHasVMEmitters(t *testing.T) {
	for _, fn := range registry.Functions() {
		if spec := lookupLFFunc(fn.Name); spec == nil {
			t.Errorf("registry function %q has no LynxFlow VM emitter", fn.Name)
		}
	}
}
