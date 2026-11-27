package service

// QuoteBoundaryLookup is an issuer archive read. Operator is the fingerprint
// authenticated by the explorer before delegation over its own mutual TLS.
type QuoteBoundaryLookup struct {
	Operator string `json:"operator"`
	Branch   string `json:"branch"`
	Sale     string `json:"sale"`
	Stream   string `json:"stream"`
	Merchant string `json:"merchant"`
	Order    string `json:"order"`
}

// QuoteBoundaryOrigin contains only the authenticated archive fields needed
// to compare this original quote with the active chain observation.
type QuoteBoundaryOrigin struct {
	QuoteBoundaryLookup
	Chain     string `json:"chain"`
	Contract  string `json:"contract"`
	Asset     string `json:"asset"`
	Manifest  string `json:"manifest"`
	ABI       string `json:"abi"`
	CodeHash  string `json:"codeHash"`
	Digest    string `json:"digest"`
	IssuedAt  int64  `json:"issuedAt"`
	ExpiresAt int64  `json:"expiresAt"`
}
