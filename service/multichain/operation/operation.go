package operation

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/jackc/pgtype"
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

func TokenFullDetailsByUserTokenIdentifiers(ctx context.Context, q *db.Queries, userID persist.DBID, tID persist.TokenIdentifiers) (TokenFullDetails, error) {
	r, err := q.GetTokenFullDetailsByUserTokenIdentifiers(ctx, db.GetTokenFullDetailsByUserTokenIdentifiersParams{
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

func InsertTokenDefinitions(ctx context.Context, q *db.Queries, tokens []db.TokenDefinition) ([]db.TokenDefinition, error) {
	// Sort to ensure consistent insertion order
	sort.SliceStable(tokens, func(i, j int) bool {
		if tokens[i].Chain != tokens[j].Chain {
			return tokens[i].Chain < tokens[j].Chain
		}
		if tokens[i].ContractAddress != tokens[j].ContractAddress {
			return tokens[i].ContractAddress < tokens[j].ContractAddress
		}
		return tokens[i].TokenID < tokens[j].TokenID
	})

	var p db.UpsertTokenDefinitionsParams
	var errors []error

	for i := range tokens {
		t := &tokens[i]
		p.DefinitionDbid = append(p.DefinitionDbid, persist.GenerateID().String())
		p.DefinitionName = append(p.DefinitionName, t.Name.String)
		p.DefinitionDescription = append(p.DefinitionDescription, t.Description.String)
		p.DefinitionTokenType = append(p.DefinitionTokenType, t.TokenType.String())
		p.DefinitionTokenID = append(p.DefinitionTokenID, t.TokenID.String())
		p.DefinitionExternalUrl = append(p.DefinitionExternalUrl, t.ExternalUrl.String)
		p.DefinitionChain = append(p.DefinitionChain, int32(t.Chain))
		appendJSONB(&p.DefinitionFallbackMedia, t.FallbackMedia, &errors)
		appendJSONB(&p.DefinitionMetadata, t.Metadata, &errors)
		p.DefinitionContractAddress = append(p.DefinitionContractAddress, t.ContractAddress.String())
		p.DefinitionContractID = append(p.DefinitionContractID, t.ContractID.String())
		p.DefinitionIsFxhash = append(p.DefinitionIsFxhash, t.IsFxhash)
		// Community memberships
		p.CommunityMembershipDbid = append(p.CommunityMembershipDbid, persist.GenerateID().String())
		p.CommunityMembershipTokenID = append(p.CommunityMembershipTokenID, t.TokenID.ToDecimalTokenID().Numeric())
		// Defer error checking until now to keep the code above from being littered with multiline "if" statements
		if len(errors) > 0 {
			return nil, errors[0]
		}
	}

	added, err := q.UpsertTokenDefinitions(ctx, p)
	if err != nil {
		return nil, err
	}

	if len(added) == 0 {
		logger.For(ctx).Infof("no new definitions added, definitions already existed in the db")
		return []db.TokenDefinition{}, nil
	}

	addedTokens := make([]db.TokenDefinition, len(added))
	for i, t := range added {
		addedTokens[i] = t.TokenDefinition
	}

	return addedTokens, nil
}

func InsertTokens(ctx context.Context, q *db.Queries, tokens []UpsertToken) (time.Time, []TokenFullDetails, error) {
	tokens = excludeZeroQuantityTokens(ctx, tokens)

	// If we're not upserting anything, we still need to return the current database time
	// since it may be used by the caller and is assumed valid if err == nil
	if len(tokens) == 0 {
		currentTime, err := q.GetCurrentTime(ctx)
		if err != nil {
			return time.Time{}, nil, err
		}
		return currentTime, []TokenFullDetails{}, nil
	}

	// Sort to ensure consistent insertion order
	sort.SliceStable(tokens, func(i, j int) bool {
		if tokens[i].Token.OwnerUserID != tokens[j].Token.OwnerUserID {
			return tokens[i].Token.OwnerUserID < tokens[j].Token.OwnerUserID
		}
		if tokens[i].Identifiers.Chain != tokens[j].Identifiers.Chain {
			return tokens[i].Identifiers.Chain < tokens[j].Identifiers.Chain
		}
		if tokens[i].Identifiers.ContractAddress != tokens[j].Identifiers.ContractAddress {
			return tokens[i].Identifiers.ContractAddress < tokens[j].Identifiers.ContractAddress
		}
		return tokens[i].Identifiers.TokenID < tokens[j].Identifiers.TokenID
	})

	var p db.UpsertTokensParams

	for i := range tokens {
		t := &tokens[i].Token
		tID := &tokens[i].Identifiers
		p.TokenDbid = append(p.TokenDbid, persist.GenerateID().String())
		p.TokenVersion = append(p.TokenVersion, t.Version.Int32)
		p.TokenCollectorsNote = append(p.TokenCollectorsNote, t.CollectorsNote.String)
		p.TokenTokenID = append(p.TokenTokenID, tID.TokenID.String())
		p.TokenQuantity = append(p.TokenQuantity, t.Quantity.String())
		p.TokenBlockNumber = append(p.TokenBlockNumber, t.BlockNumber.Int64)
		p.TokenOwnerUserID = append(p.TokenOwnerUserID, t.OwnerUserID.String())
		appendDBIDList(&p.TokenOwnedByWallets, t.OwnedByWallets, &p.TokenOwnedByWalletsStartIdx, &p.TokenOwnedByWalletsEndIdx)
		p.TokenChain = append(p.TokenChain, int32(tID.Chain))
		p.TokenContractAddress = append(p.TokenContractAddress, tID.ContractAddress.String())
		p.TokenIsCreatorToken = append(p.TokenIsCreatorToken, t.IsCreatorToken)
		p.TokenContractID = append(p.TokenContractID, t.ContractID.String())
	}

	added, err := q.UpsertTokens(ctx, p)
	if err != nil {
		return time.Time{}, nil, err
	}

	if len(added) == 0 {
		logger.For(ctx).Infof("no new tokens added, tokens already existed in the db")
		return time.Time{}, []TokenFullDetails{}, nil
	}

	addedTokens := make([]TokenFullDetails, len(added))
	for i, t := range added {
		addedTokens[i] = TokenFullDetails{
			Instance:   t.Token,
			Contract:   t.Contract,
			Definition: t.TokenDefinition,
		}
	}

	return addedTokens[0].Instance.LastSynced, addedTokens, nil
}

func excludeZeroQuantityTokens(ctx context.Context, tokens []UpsertToken) []UpsertToken {
	return util.Filter(tokens, func(t UpsertToken) bool {
		if t.Token.Quantity == "" || t.Token.Quantity == "0" {
			logger.For(ctx).Warnf("Token(chain=%d, address=%s, tokenID=%s) has 0 quantity", t.Identifiers.Chain, t.Identifiers.ContractAddress, t.Identifiers.TokenID)
			return false
		}
		return true
	}, false)
}

func appendIndices(startIndices *[]int32, endIndices *[]int32, entryLength int) {
	// Postgres uses 1-based indexing
	startIndex := int32(1)
	if len(*endIndices) > 0 {
		startIndex = (*endIndices)[len(*endIndices)-1] + 1
	}
	*startIndices = append(*startIndices, startIndex)
	*endIndices = append(*endIndices, startIndex+int32(entryLength)-1)
}

func appendJSONB(dest *[]pgtype.JSONB, src any, errs *[]error) error {
	jsonb, err := persist.ToJSONB(src)
	if err != nil {
		*errs = append(*errs, err)
		return err
	}
	*dest = append(*dest, jsonb)
	return nil
}

func appendDBIDList(dest *[]string, src []persist.DBID, startIndices, endIndices *[]int32) {
	for _, id := range src {
		*dest = append(*dest, id.String())
	}
	appendIndices(startIndices, endIndices, len(src))
}
