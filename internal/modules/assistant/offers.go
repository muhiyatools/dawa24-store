package assistant

// OfferQuery asks what a buyer can order for one of its branches.
type OfferQuery struct {
	Search string
	// BranchID is a verified branch id, or zero for the caller's default
	// buying branch.
	BranchID       int64
	OnlyDiscounted bool
	Limit          int
}

// BuyableOffer is one supplier listing as the buying branch sees it right now.
// Orderable and MaxQuantity come from the same availability rule the cart and
// checkout apply, so the assistant cannot promise what checkout would refuse.
type BuyableOffer struct {
	VariantID   int64  `json:"-"`
	ProductID   int64  `json:"-"`
	Ref         string `json:"ref"`
	Product     string `json:"product"`
	Scientific  string `json:"scientific_name,omitempty"`
	Supplier    string `json:"supplier"`
	Price       string `json:"price"`
	PublicPrice string `json:"public_price,omitempty"`
	Discount    int    `json:"discount_percent,omitempty"`
	MinQuantity int    `json:"min_quantity,omitempty"`
	Orderable   bool   `json:"orderable"`
	MaxQuantity int    `json:"max_quantity,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Expiry      string `json:"expiry,omitempty"`
}

// OfferResult is a page of buyable offers for one branch.
type OfferResult struct {
	Branch string         `json:"branch"`
	Total  int            `json:"total"`
	Offers []BuyableOffer `json:"offers"`
}
