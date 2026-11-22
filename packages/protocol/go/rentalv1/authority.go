package rentalv1

// AuthorityPath is the private current-enrollment lookup from P07 to P05.
const AuthorityPath = "/v2/private/rental-generation/authority"

type AuthorityRequest struct {
	Stream            string `json:"streamId"`
	StreamGeneration  string `json:"streamGeneration"`
	Branch            string `json:"branch"`
	Board             string `json:"board"`
	Generation        string `json:"generation"`
	Challenge         string `json:"challenge"`
	CallerFingerprint string `json:"callerFingerprint"`
}

type AuthorityResponse struct {
	AuthorityRequest
	Registration   string `json:"registration"`
	Measurement    string `json:"measurement"`
	Deployment     string `json:"deployment"`
	ChainID        uint64 `json:"chainId"`
	Settlement     string `json:"settlement"`
	Account        string `json:"account"`
	BoardSigner    string `json:"boardSigner"`
	Operator       string `json:"operator"`
	RegisteredAt   int64  `json:"registeredAt"`
	ChallengeUntil int64  `json:"challengeUntil"`
	ValidUntil     int64  `json:"validUntil"`
}
