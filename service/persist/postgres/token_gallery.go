package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/jackc/pgx/v4"

	db "github.com/mikeydub/go-gallery/db/gen/coredb"
	"github.com/mikeydub/go-gallery/service/logger"
	"github.com/mikeydub/go-gallery/service/persist"
	"github.com/mikeydub/go-gallery/util"
)

type TokenFullDetails struct {
	Instance   db.Token
	Contract   db.Contract
	Definition db.TokenDefinition
}

type TokenFullDetailsRepository struct {
	queries *db.Queries
}

func NewTokenFullDetailsRepository(queries *db.Queries) *TokenFullDetailsRepository {
	return &TokenFullDetailsRepository{queries: queries}
}

func (t *TokenFullDetailsRepository) GetByUserTokenIdentifiers(ctx context.Context, userID persist.DBID, tID persist.TokenIdentifiers) (TokenFullDetails, error) {
	r, err := t.queries.GetTokenFullDetailsByUserTokenIdentifiers(ctx, db.GetTokenFullDetailsByUserTokenIdentifiersParams{
		OwnerUserID:     userID,
		Chain:           tID.Chain,
		ContractAddress: tID.ContractAddress,
		TokenID:         tID.TokenID,
	})
	if err != nil {
		if err == pgx.ErrNoRows {
			return TokenFullDetails{}, persist.ErrTokenNotFoundByUserTokenIdentifers{UserID: userID, Token: tID}
		}
		return TokenFullDetails{}, err
	}
	return TokenFullDetails{
		Instance:   r.Token,
		Contract:   r.Contract,
		Definition: r.TokenDefinition,
	}, nil
}

// GetByUserID gets all tokens for a user
func (t *TokenFullDetailsRepository) GetByUserID(ctx context.Context, userID persist.DBID) ([]TokenFullDetails, error) {
	r, err := t.queries.GetTokenFullDetailsByUserId(ctx, userID)
	if err != nil {
		return nil, err
	}
	tokens := util.MapWithoutError(r, func(r db.GetTokenFullDetailsByUserIdRow) TokenFullDetails {
		return TokenFullDetails{
			Instance:   r.Token,
			Contract:   r.Contract,
			Definition: r.TokenDefinition,
		}
	})
	return tokens, nil
}

// GetByContractID gets all tokens for a contract
func (t *TokenFullDetailsRepository) GetByContractID(ctx context.Context, contractID persist.DBID) ([]TokenFullDetails, error) {
	r, err := t.queries.GetTokenFullDetailsByContractId(ctx, contractID)
	if err != nil {
		return nil, err
	}
	tokens := util.MapWithoutError(r, func(r db.GetTokenFullDetailsByContractIdRow) TokenFullDetails {
		return TokenFullDetails{
			Instance:   r.Token,
			Contract:   r.Contract,
			Definition: r.TokenDefinition,
		}
	})
	return tokens, nil
}

type TokenUpsertParams struct {
	SetCreatorFields bool
	SetHolderFields  bool

	// If OptionalDelete is nil, no delete will be performed
	OptionalDelete *TokenUpsertDeletionParams
}

type TokenUpsertDeletionParams struct {
	DeleteCreatorStatus bool
	DeleteHolderStatus  bool
	OnlyFromUserID      *persist.DBID
	OnlyFromContracts   []persist.DBID
	OnlyFromChains      []persist.Chain
}

func (d *TokenUpsertDeletionParams) ToParams(upsertTime time.Time) db.DeleteTokensBeforeTimestampParams {
	userID := ""
	if d.OnlyFromUserID != nil {
		userID = d.OnlyFromUserID.String()
	}

	chains := util.MapWithoutError(d.OnlyFromChains, func(c persist.Chain) int32 { return int32(c) })
	return db.DeleteTokensBeforeTimestampParams{
		RemoveCreatorStatus: d.DeleteCreatorStatus,
		RemoveHolderStatus:  d.DeleteHolderStatus,
		OnlyFromUserID:      sql.NullString{String: userID, Valid: d.OnlyFromUserID != nil},
		OnlyFromChains:      chains,
		OnlyFromContractIds: util.StringersToStrings(d.OnlyFromContracts),
		Timestamp:           upsertTime,
	}
}

type UpsertToken struct {
	Token db.Token
	// Identifiers aren't saved to the database, but are used for joining the token to its definitions
	Identifiers persist.TokenIdentifiers
}

func (t *TokenFullDetailsRepository) BulkUpsert(ctx context.Context, tokens []UpsertToken, definitions []db.TokenDefinition, setCreatorFields bool, setHolderFields bool) (time.Time, []TokenFullDetails, error) {
	tokens = t.excludeZeroQuantityTokens(ctx, tokens)

	// If we're not upserting anything, we still need to return the current database time
	// since it may be used by the caller and is assumed valid if err == nil
	if len(tokens) == 0 {
		currentTime, err := t.queries.GetCurrentTime(ctx)
		if err != nil {
			return time.Time{}, nil, err
		}
		return currentTime, []TokenFullDetails{}, nil
	}

	params := db.UpsertTokensParams{
		SetCreatorFields:    setCreatorFields,
		SetHolderFields:     setHolderFields,
		TokenOwnedByWallets: []string{},
	}

	var errors []error

	for i := range definitions {
		d := &definitions[i]
		params.DefinitionDbid = append(params.DefinitionDbid, persist.GenerateID().String())
		params.DefinitionName = append(params.DefinitionName, d.Name.String)
		params.DefinitionDescription = append(params.DefinitionDescription, d.Description.String)
		params.DefinitionTokenType = append(params.DefinitionTokenType, d.TokenType.String())
		params.DefinitionTokenID = append(params.DefinitionTokenID, d.TokenID.String())
		params.DefinitionExternalUrl = append(params.DefinitionExternalUrl, d.ExternalUrl.String)
		params.DefinitionChain = append(params.DefinitionChain, int32(d.Chain))
		appendJSONB(&params.DefinitionFallbackMedia, d.FallbackMedia, &errors)
		appendJSONB(&params.DefinitionMetadata, d.Metadata, &errors)
		params.DefinitionContractAddress = append(params.DefinitionContractAddress, d.ContractAddress.String())
		params.DefinitionContractID = append(params.DefinitionContractID, d.ContractID.String())
		// Defer error checking until now to keep the code above from being
		// littered with multiline "if" statements
		if len(errors) > 0 {
			return time.Time{}, nil, errors[0]
		}
	}

	for i := range tokens {
		t := &tokens[i].Token
		tID := &tokens[i].Identifiers
		params.TokenDbid = append(params.TokenDbid, persist.GenerateID().String())
		params.TokenVersion = append(params.TokenVersion, t.Version.Int32)
		params.TokenCollectorsNote = append(params.TokenCollectorsNote, t.CollectorsNote.String)
		params.TokenTokenID = append(params.TokenTokenID, tID.TokenID.String())
		params.TokenQuantity = append(params.TokenQuantity, t.Quantity.String())
		params.TokenBlockNumber = append(params.TokenBlockNumber, t.BlockNumber.Int64)
		params.TokenOwnerUserID = append(params.TokenOwnerUserID, t.OwnerUserID.String())
		appendDBIDList(&params.TokenOwnedByWallets, t.OwnedByWallets, &params.TokenOwnedByWalletsStartIdx, &params.TokenOwnedByWalletsEndIdx)
		params.TokenChain = append(params.TokenChain, int32(tID.Chain))
		params.TokenContractAddress = append(params.TokenContractAddress, tID.ContractAddress.String())
		params.TokenIsCreatorToken = append(params.TokenIsCreatorToken, t.IsCreatorToken)
		params.TokenContractID = append(params.TokenContractID, t.ContractID.String())
		// Defer error checking until now to keep the code above from being
		// littered with multiline "if" statements
		if len(errors) > 0 {
			return time.Time{}, nil, errors[0]
		}
	}

	upserted, err := t.queries.UpsertTokens(ctx, params)
	if err != nil {
		return time.Time{}, nil, err
	}

	upsertedTokens := util.MapWithoutError(upserted, func(r db.UpsertTokensRow) TokenFullDetails {
		return TokenFullDetails{
			Instance:   r.Token,
			Contract:   r.Contract,
			Definition: r.TokenDefinition,
		}
	})

	return upserted[0].Token.LastSynced, upsertedTokens, nil
}

func (t *TokenFullDetailsRepository) excludeZeroQuantityTokens(ctx context.Context, tokens []UpsertToken) []UpsertToken {
	return util.Filter(tokens, func(t UpsertToken) bool {
		if t.Token.Quantity == "" || t.Token.Quantity == "0" {
			logger.For(ctx).Warnf("Token(chain=%d, address=%s, tokenID=%s) has 0 quantity", t.Identifiers.Chain, t.Identifiers.ContractAddress, t.Identifiers.TokenID)
			return false
		}
		return true
	}, false)
}
