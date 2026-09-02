package compare

// The market benchmark used to be implemented here.
//
// It is now market_benchmark.go, which compares against the whole market rather
// than the hundred rows ListMarketDiscounts returns for a Limit of 50000, and
// which groups the market by catalogue product rather than by three text keys
// derived from the raw product name. See market_dataset.go for why the old
// screen reported nearly every row as حصري.
