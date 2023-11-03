package persist

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// TokenIdentifiers represents a unique identifier for a token
type TokenIdentifiers struct {
	TokenID         TokenID `json:"token_id"`
	ContractAddress Address `json:"contract_address"`
	Chain           Chain   `json:"chain"`
}

type TokenUniqueIdentifiers struct {
	Chain           Chain   `json:"chain"`
	ContractAddress Address `json:"contract_address"`
	TokenID         TokenID `json:"token_id"`
	OwnerAddress    Address `json:"owner_address"`
}

// ContractIdentifiers represents a unique identifier for a contract
type ContractIdentifiers struct {
	ContractAddress Address `json:"contract_address"`
	Chain           Chain   `json:"chain"`
}

// NewTokenIdentifiers creates a new token identifiers
func NewTokenIdentifiers(pContractAddress Address, pTokenID TokenID, pChain Chain) TokenIdentifiers {
	return TokenIdentifiers{
		TokenID:         TokenID(pTokenID.BigInt().Text(16)),
		ContractAddress: Address(pChain.NormalizeAddress(pContractAddress)),
		Chain:           pChain,
	}
}

func (t TokenIdentifiers) String() string {
	return fmt.Sprintf("%s+%s+%d", t.Chain.NormalizeAddress(t.ContractAddress), t.TokenID, t.Chain)
}

// Value implements the driver.Valuer interface
func (t TokenIdentifiers) Value() (driver.Value, error) {
	return t.String(), nil
}

// Scan implements the database/sql Scanner interface for the TokenIdentifiers type
func (t *TokenIdentifiers) Scan(i interface{}) error {
	if i == nil {
		*t = TokenIdentifiers{}
		return nil
	}
	res := strings.Split(i.(string), "+")
	if len(res) != 2 {
		return fmt.Errorf("invalid token identifiers: %v - %T", i, i)
	}
	chain, err := strconv.Atoi(res[2])
	if err != nil {
		return err
	}
	*t = TokenIdentifiers{
		TokenID:         TokenID(res[1]),
		ContractAddress: Address(res[0]),
		Chain:           Chain(chain),
	}
	return nil
}

func (t TokenUniqueIdentifiers) String() string {
	return fmt.Sprintf("%s+%s+%s+%d", t.Chain.NormalizeAddress(t.ContractAddress), t.TokenID, t.Chain.NormalizeAddress(t.OwnerAddress), t.Chain)
}

func TokenUniqueIdentifiersFromString(s string) (TokenUniqueIdentifiers, error) {
	res := strings.Split(s, "+")
	if len(res) != 4 {
		return TokenUniqueIdentifiers{}, fmt.Errorf("invalid token unique identifiers: %v", s)
	}
	chain, err := strconv.Atoi(res[3])
	if err != nil {
		return TokenUniqueIdentifiers{}, err
	}
	return TokenUniqueIdentifiers{
		TokenID:         TokenID(res[1]),
		ContractAddress: Address(res[0]),
		Chain:           Chain(chain),
		OwnerAddress:    Address(res[2]),
	}, nil
}

// NewContractIdentifiers creates a new contract identifiers
func NewContractIdentifiers(pContractAddress Address, pChain Chain) ContractIdentifiers {
	return ContractIdentifiers{
		ContractAddress: pContractAddress,
		Chain:           pChain,
	}
}
