package persist

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// TokenIdentifiers represents a unique identifier for a token
type TokenIdentifiers struct {
	TokenID         HexTokenID `json:"token_id"`
	ContractAddress Address    `json:"contract_address"`
	Chain           Chain      `json:"chain"`
}

func (t TokenIdentifiers) String() string {
	return fmt.Sprintf("token(chain=%s, contract=%s, tokenID=%s)", t.Chain, t.ContractAddress, t.TokenID.ToDecimalTokenID())
}

// NewTokenIdentifiers creates a new token identifiers
func NewTokenIdentifiers(pContractAddress Address, pTokenID HexTokenID, pChain Chain) TokenIdentifiers {
	return TokenIdentifiers{
		TokenID:         HexTokenID(pTokenID.BigInt().Text(16)),
		ContractAddress: pContractAddress,
		Chain:           pChain,
	}
}

type TokenUniqueIdentifiers struct {
	Chain           Chain      `json:"chain"`
	ContractAddress Address    `json:"contract_address"`
	TokenID         HexTokenID `json:"token_id"`
	OwnerAddress    Address    `json:"owner_address"`
}

func (t TokenUniqueIdentifiers) String() string {
	return fmt.Sprintf("token(chain=%s, contract=%s, tokenID=%s, owner=%s)", t.Chain, t.ContractAddress, t.TokenID.ToDecimalTokenID(), t.OwnerAddress)
}

func (t TokenUniqueIdentifiers) AsJSONKey() string {
	return fmt.Sprintf("%s+%s+%s+%d", t.ContractAddress, t.TokenID, t.OwnerAddress, t.Chain)
}

func (t *TokenUniqueIdentifiers) FromJSONKey(key string) error {
	res := strings.Split(key, "+")
	if len(res) != 4 {
		return fmt.Errorf("invalid token unique identifiers: %v", key)
	}
	chain, err := strconv.Atoi(res[3])
	if err != nil {
		return err
	}
	t.Chain = Chain(chain)
	t.ContractAddress = Address(res[0])
	t.TokenID = HexTokenID(res[1])
	t.OwnerAddress = Address(res[2])
	return nil
}

func (t *TokenUniqueIdentifiers) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"chain":            t.Chain,
		"contract_address": t.ContractAddress,
		"token_id":         t.TokenID,
		"owner_address":    t.OwnerAddress,
	})
}

func (t *TokenUniqueIdentifiers) UnmarshalJSON(data []byte) error {
	var tmp struct {
		Chain           Chain      `json:"chain"`
		ContractAddress Address    `json:"contract_address"`
		TokenID         HexTokenID `json:"token_id"`
		OwnerAddress    Address    `json:"owner_address"`
	}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	t.Chain = tmp.Chain
	t.ContractAddress = tmp.ContractAddress
	t.TokenID = tmp.TokenID
	t.OwnerAddress = tmp.OwnerAddress
	return nil
}

// ContractIdentifiers represents a unique identifier for a contract
type ContractIdentifiers struct {
	ContractAddress Address `json:"contract_address"`
	Chain           Chain   `json:"chain"`
}

func (c ContractIdentifiers) String() string {
	return fmt.Sprintf("contract(chain=%s, address=%s)", c.Chain, c.ContractAddress)
}

// NewContractIdentifiers creates a new contract identifiers
func NewContractIdentifiers(pContractAddress Address, pChain Chain) ContractIdentifiers {
	return ContractIdentifiers{
		ContractAddress: pContractAddress,
		Chain:           pChain,
	}
}

type ErrContractCreatorNotFound struct {
	ContractID DBID
}

func (e ErrContractCreatorNotFound) Error() string {
	return fmt.Sprintf("ContractCreator not found for contractID %s", e.ContractID)
}
