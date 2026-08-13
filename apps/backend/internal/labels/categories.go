package labels

type EntityCategory string

const (
	CategoryExchange          EntityCategory = "exchange"
	CategoryBridge            EntityCategory = "bridge"
	CategoryMerchant          EntityCategory = "merchant"
	CategoryOTC               EntityCategory = "otc"
	CategoryMixer             EntityCategory = "mixer"
	CategorySanctionedService EntityCategory = "sanctioned-service"
	CategoryHighRiskService   EntityCategory = "high-risk-service"
	CategoryDeFiService       EntityCategory = "defi-service"
	CategoryBurnAddress       EntityCategory = "burn-address"
)

func ValidCategory(category EntityCategory) bool {
	switch category {
	case CategoryExchange, CategoryBridge, CategoryMerchant, CategoryOTC, CategoryMixer,
		CategorySanctionedService, CategoryHighRiskService, CategoryDeFiService, CategoryBurnAddress:
		return true
	default:
		return false
	}
}
