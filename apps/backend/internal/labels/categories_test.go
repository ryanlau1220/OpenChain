package labels

import "testing"

func TestEntityCategoriesAndEvidenceURLsAreExplicit(t *testing.T) {
	for _, category := range []EntityCategory{
		CategoryExchange, CategoryBridge, CategoryMerchant, CategoryOTC, CategoryMixer,
		CategorySanctionedService, CategoryHighRiskService,
	} {
		if !ValidCategory(category) {
			t.Fatalf("expected category %q to be valid", category)
		}
	}
	if ValidCategory("unverified") || evidenceURL("javascript:alert(1)") || !evidenceURL("https://example.test/proof") {
		t.Fatal("entity category or evidence URL validation is incorrect")
	}
}
