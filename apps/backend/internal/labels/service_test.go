package labels

import "testing"

func TestSeedLabelsArePublicMainnetEvidenceBacked(t *testing.T) {
	items, err := SeedLabels()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		t.Fatal("expected curated labels")
	}
	for _, item := range items {
		if item.Network != ethereumMainnet || !ValidCategory(item.Category) || item.Visibility != "public" || item.EvidenceURL == "" || item.EvidenceSnapshot == "" || item.Source == "" || item.SourceVersion == "" || item.ReviewState != "approved" || item.TrustTier == 0 || item.ValidFrom.IsZero() || item.CreatedAt.IsZero() {
			t.Fatalf("label lacks required provenance: %#v", item)
		}
	}
}
