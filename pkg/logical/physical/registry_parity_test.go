package physical

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/registry"
)

func TestLynxFlowAggregateRegistryHasPhysicalMapping(t *testing.T) {
	for _, ag := range registry.Aggregates() {
		if mapped := aggNameMapping[ag.Name]; mapped == "" {
			t.Errorf("registry aggregate %q has no physical mapping", ag.Name)
		}
	}
}
