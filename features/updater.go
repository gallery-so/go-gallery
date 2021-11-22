package features

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/mikeydub/go-gallery/contracts"
	"github.com/mikeydub/go-gallery/persist"
	"github.com/sirupsen/logrus"
)

// eventHash represents an event keccak256 hash
type eventHash string

type erc20Transfer struct {
	From   common.Address
	To     common.Address
	Tokens *big.Int
}

// TODO ERC-20

const (
	// transferEventHash represents the keccak256 hash of Transfer(address,address,uint256)
	transferEventHash eventHash = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
	// transferSingleEventHash represents the keccak256 hash of TransferSingle(address,address,address,uint256,uint256)
	transferSingleEventHash eventHash = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
	// transferBatchEventHash represents the keccak256 hash of TransferBatch(address,address,address,uint256[],uint256[])
	transferBatchEventHash eventHash = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"
)

func trackFeatures(pCtx context.Context, userRepo persist.UserRepository, featureRepo persist.FeatureFlagRepository, accessRepo persist.AccessRepository, ethClient *ethclient.Client) {
	incomingLogs := make(chan types.Log)

	allFeatures, err := featureRepo.GetAll(pCtx)
	if err != nil {
		panic(err)
	}
	current, err := ethClient.BlockNumber(pCtx)
	if err != nil {
		panic(err)
	}

	addresses := []common.Address{}
	for _, feature := range allFeatures {
		address, _ := feature.RequiredToken.GetParts()
		addresses = append(addresses, address.Address())
	}
	topics := [][]common.Hash{{common.HexToHash(string(transferEventHash)), common.HexToHash(string(transferSingleEventHash)), common.HexToHash(string(transferBatchEventHash))}}

	q := ethereum.FilterQuery{
		FromBlock: big.NewInt(0).SetUint64(current - 1),
		Addresses: addresses,
		Topics:    topics,
	}
	sub, err := ethClient.SubscribeFilterLogs(pCtx, q, incomingLogs)
	if err != nil {
		panic(err)
	}
	for {
		select {
		case log := <-incomingLogs:
			logrus.Infof("Got log at: %d", log.BlockNumber)
			go func() {
				context, cancel := context.WithTimeout(pCtx, time.Minute*1)
				defer cancel()
				err := processIncomingLog(context, userRepo, accessRepo, log)
				if err != nil {
					logrus.Errorf("Error processing log: %s - log: %+v", err.Error(), log)
				}
			}()
		case err := <-sub.Err():
			panic(fmt.Sprintf("error in log subscription: %s", err))
		}
	}

}

func processIncomingLog(pCtx context.Context, userRepo persist.UserRepository, accessRepo persist.AccessRepository, pLog types.Log) error {

	switch {
	case strings.EqualFold(pLog.Topics[0].Hex(), string(transferEventHash)):

		if len(pLog.Topics) < 3 {
			return fmt.Errorf("invalid log: %+v", pLog)
		}

		from := persist.Address(pLog.Topics[1].Hex())
		to := persist.Address(pLog.Topics[2].Hex())
		blockNumber := persist.BlockNumber(pLog.BlockNumber)
		contract := persist.Address(pLog.Address.Hex())

		if len(pLog.Topics) == 3 {

			contractAbi, err := abi.JSON(strings.NewReader(string(contracts.IERC20ABI)))
			if err != nil {
				return err
			}

			transferEvent := erc20Transfer{}

			err = contractAbi.UnpackIntoInterface(&transferEvent, "Transfer", pLog.Data)
			if err != nil {
				return err
			}

			ti := persist.NewTokenIdentifiers(contract, persist.TokenID("0"))

			fromUser, err := userRepo.GetByAddress(pCtx, from)
			if err == nil {
				currentAccess, err := accessRepo.GetByUserID(pCtx, fromUser.ID)
				if err == nil {
					currentAccess.RequiredTokensOwned[ti] -= transferEvent.Tokens.Uint64()
					accessRepo.UpdateRequiredTokensByUserID(pCtx, fromUser.ID, currentAccess.RequiredTokensOwned, blockNumber)
				}
			}
			toUser, err := userRepo.GetByAddress(pCtx, to)
			if err == nil {
				currentAccess, err := accessRepo.GetByUserID(pCtx, toUser.ID)
				if err == nil {
					currentAccess.RequiredTokensOwned[ti] += transferEvent.Tokens.Uint64()
					accessRepo.UpdateRequiredTokensByUserID(pCtx, toUser.ID, currentAccess.RequiredTokensOwned, blockNumber)
				}
			}

		} else if len(pLog.Topics) == 4 {
			tokenID := persist.TokenID(pLog.Topics[3].Hex())

			ti := persist.NewTokenIdentifiers(contract, tokenID)

			fromUser, err := userRepo.GetByAddress(pCtx, from)
			if err == nil {
				accessRepo.UpdateRequiredTokensByUserID(pCtx, fromUser.ID, map[persist.TokenIdentifiers]uint64{
					ti: 0,
				}, blockNumber)
			}
			toUser, err := userRepo.GetByAddress(pCtx, to)
			if err == nil {
				accessRepo.UpdateRequiredTokensByUserID(pCtx, toUser.ID, map[persist.TokenIdentifiers]uint64{
					ti: 1,
				}, blockNumber)
			}
		} else {
			return fmt.Errorf("invalid log: %+v", pLog)
		}

	case strings.EqualFold(pLog.Topics[0].Hex(), string(transferSingleEventHash)):
		if len(pLog.Topics) < 4 {
			return fmt.Errorf("invalid log: %+v", pLog)
		}

		from := persist.Address(pLog.Topics[2].Hex())
		to := persist.Address(pLog.Topics[3].Hex())
		tokenID := persist.TokenID(common.BytesToHash(pLog.Data[:len(pLog.Data)/2]).Hex())
		amount := common.BytesToHash(pLog.Data[len(pLog.Data)/2:]).Big().Uint64()
		blockNumber := persist.BlockNumber(pLog.BlockNumber)
		contract := persist.Address(pLog.Address.Hex())

		ti := persist.NewTokenIdentifiers(contract, tokenID)
		fromUser, err := userRepo.GetByAddress(pCtx, from)
		if err == nil {
			currentAccess, err := accessRepo.GetByUserID(pCtx, fromUser.ID)
			if err == nil {
				currentAccess.RequiredTokensOwned[ti] -= amount
				accessRepo.UpdateRequiredTokensByUserID(pCtx, fromUser.ID, currentAccess.RequiredTokensOwned, blockNumber)
			}
		}
		toUser, err := userRepo.GetByAddress(pCtx, to)
		if err == nil {
			currentAccess, err := accessRepo.GetByUserID(pCtx, toUser.ID)
			if err == nil {
				currentAccess.RequiredTokensOwned[ti] += amount
				accessRepo.UpdateRequiredTokensByUserID(pCtx, toUser.ID, currentAccess.RequiredTokensOwned, blockNumber)
			}
		}

	case strings.EqualFold(pLog.Topics[0].Hex(), string(transferBatchEventHash)):
		if len(pLog.Topics) < 4 {
			return fmt.Errorf("invalid log: %+v", pLog)
		}
		from := persist.Address(pLog.Topics[2].Hex())
		to := persist.Address(pLog.Topics[3].Hex())
		amountOffset := len(pLog.Data) / 2
		total := amountOffset / 64
		contract := persist.Address(pLog.Address.Hex())
		blockNumber := persist.BlockNumber(pLog.BlockNumber)

		var fromAccess, toAccess *persist.Access

		fromUser, err := userRepo.GetByAddress(pCtx, from)
		if err == nil {
			currentAccess, err := accessRepo.GetByUserID(pCtx, fromUser.ID)
			if err != nil {
				return err
			}
			fromAccess = currentAccess
		}
		toUser, err := userRepo.GetByAddress(pCtx, to)
		if err == nil {
			currentAccess, err := accessRepo.GetByUserID(pCtx, toUser.ID)
			if err != nil {
				return err
			}
			toAccess = currentAccess
		}

		for j := 0; j < total; j++ {

			tokenID := persist.TokenID(common.BytesToHash(pLog.Data[j*64 : (j+1)*64]).Hex())
			amount := common.BytesToHash(pLog.Data[(amountOffset)+(j*64) : (amountOffset)+((j+1)*64)]).Big().Uint64()

			ti := persist.NewTokenIdentifiers(contract, tokenID)

			if fromAccess != nil {
				fromAccess.RequiredTokensOwned[ti] -= amount
			}

			if toAccess != nil {
				toAccess.RequiredTokensOwned[ti] += amount
			}

		}
		if fromAccess != nil {
			accessRepo.UpdateRequiredTokensByUserID(pCtx, fromUser.ID, fromAccess.RequiredTokensOwned, blockNumber)
		}

		if toAccess != nil {
			accessRepo.UpdateRequiredTokensByUserID(pCtx, toUser.ID, toAccess.RequiredTokensOwned, blockNumber)
		}
	default:
		logrus.WithFields(logrus.Fields{"address": pLog.Address, "block": pLog.BlockNumber, "event_type": pLog.Topics[0]}).Warn("unknown event")
	}

	return nil
}
