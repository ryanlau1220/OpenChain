package labels

import (
	"context"
	"testing"
)

func TestLabelRegistryLoadsSeedLabels(t *testing.T) {
	registry := NewRegistry()
	labels := registry.GetLabels(context.Background(), "0xee567fe1712faf6149d80da1e6934e354124cfe3")
	if len(labels) == 0 || labels[0].Label != "Uniswap V2 Router" { t.Fatalf("labels = %#v", labels) }
}

func TestLabelSearchHonorsLimit(t *testing.T) {
	labels := NewRegistry().SearchLabels(context.Background(), "", "", 1)
	if len(labels) != 1 { t.Fatalf("labels = %d", len(labels)) }
}
