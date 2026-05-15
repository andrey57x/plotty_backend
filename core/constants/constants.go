package constants

const (
	AIJobTypeSpellcheck      = "spellcheck"
	AIJobTypeImageGeneration = "image_generation"
	AIJobTypeLogicCheck      = "logic_check"

	AIJobStatusQueued     = "queued"
	AIJobStatusProcessing = "processing"
	AIJobStatusCompleted  = "completed"
	AIJobStatusFailed     = "failed"
)

const (
	// Порог слов в запросе, после которого включается семантический поиск
	SemanticSearchMinWords = 6
)

const (
	TagCategoryDirectionality = "directionality"
	TagCategoryGenre          = "genre"
	TagCategoryWarning        = "warning"
	TagCategoryRating         = "rating"
	TagCategorySize           = "size"
	TagCategoryCompletion     = "completion"
)

const (
	CreditCostImageGen   = 3
	CreditCostLogicCheck = 2
	CreditCostCanonCheck = 2
	CreditInitialBalance = 50
)

const (
	CreditTxTypeUsage    = "usage"
	CreditTxTypePurchase = "purchase"

	CreditTxStatusCompleted = "completed"
	CreditTxStatusPending   = "pending"
)

type CreditPackage struct {
	ID           int `json:"id"`
	Credits      int `json:"credits"`
	PriceKopecks int `json:"priceKopecks"`
}

var CreditPackages = []CreditPackage{
	{ID: 1, Credits: 50, PriceKopecks: 2900},
	{ID: 2, Credits: 150, PriceKopecks: 7900},
	{ID: 3, Credits: 500, PriceKopecks: 21900},
}
