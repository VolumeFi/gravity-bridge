package types

import (
	"math/big"
	"strings"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

// GetCheckpoint gets the checkpoint signature from the given outgoing tx batch
func (b BatchTx) GetCheckpoint(gravityIDstring string) ([]byte, error) {

	abi, err := abi.JSON(strings.NewReader(OutgoingBatchTxCheckpointABIJSON))
	if err != nil {
		return nil, sdkerrors.Wrap(err, "bad ABI definition in code")
	}

	// the contract argument is not a arbitrary length array but a fixed length 32 byte
	// array, therefore we have to utf8 encode the string (the default in this case) and
	// then copy the variable length encoded data into a fixed length array. This function
	// will panic if gravityId is too long to fit in 32 bytes
	gravityID, err := strToFixByteArray(gravityIDstring)
	if err != nil {
		panic(err)
	}

	// Create the methodName argument which salts the signature
	methodNameBytes := []uint8("transactionBatch")
	var batchMethodName [32]uint8
	copy(batchMethodName[:], methodNameBytes[:])

	// Run through the elements of the batch and serialize them
	txAmounts := make([]*big.Int, len(b.Transactions))
	txDestinations := make([]common.Address, len(b.Transactions))
	txFees := make([]*big.Int, len(b.Transactions))
	for i, tx := range b.Transactions {
		txAmounts[i] = tx.Erc20Token.Amount.BigInt()
		txDestinations[i] = common.HexToAddress(tx.EthereumRecipient)
		txFees[i] = tx.Erc20Fee.Amount.BigInt()
	}

	// the methodName needs to be the same as the 'name' above in the checkpointAbiJson
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	abiEncodedBatch, err := abi.Pack("submitBatch",
		gravityID,
		batchMethodName,
		txAmounts,
		txDestinations,
		txFees,
		big.NewInt(int64(b.Nonce)),
		common.HexToAddress(b.TokenContract),
		big.NewInt(int64(b.Timeout)),
	)

	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if err != nil {
		return nil, sdkerrors.Wrap(err, "packing checkpoint")
	}

	// we hash the resulting encoded bytes discarding the first 4 bytes these 4 bytes are the constant
	// method name 'checkpoint'. If you where to replace the checkpoint constant in this code you would
	// then need to adjust how many bytes you truncate off the front to get the output of abi.encode()
	return crypto.Keccak256Hash(abiEncodedBatch[4:]).Bytes(), nil
}

// GetCheckpoint gets the checkpoint signature from the given outgoing tx batch
func (c LogicCallTx) GetCheckpoint(gravityIDstring string) ([]byte, error) {

	abi, err := abi.JSON(strings.NewReader(OutgoingLogicCallABIJSON))
	if err != nil {
		return nil, sdkerrors.Wrap(err, "bad ABI definition in code")
	}

	// Create the methodName argument which salts the signature
	methodNameBytes := []uint8("logicCall")
	var logicCallMethodName [32]uint8
	copy(logicCallMethodName[:], methodNameBytes[:])

	// the contract argument is not a arbitrary length array but a fixed length 32 byte
	// array, therefore we have to utf8 encode the string (the default in this case) and
	// then copy the variable length encoded data into a fixed length array. This function
	// will panic if gravityId is too long to fit in 32 bytes
	gravityID, err := strToFixByteArray(gravityIDstring)
	if err != nil {
		panic(err)
	}

	// Run through the elements of the logic call and serialize them
	transferAmounts := make([]*big.Int, len(c.Tokens))
	transferTokenContracts := make([]common.Address, len(c.Tokens))
	feeAmounts := make([]*big.Int, len(c.Fees))
	feeTokenContracts := make([]common.Address, len(c.Fees))
	for i, tx := range c.Tokens {
		transferAmounts[i] = tx.Amount.BigInt()
		transferTokenContracts[i] = common.HexToAddress(tx.Denom)
	}
	for i, tx := range c.Fees {
		feeAmounts[i] = tx.Amount.BigInt()
		feeTokenContracts[i] = common.HexToAddress(tx.Denom)
	}
	payload := make([]byte, len(c.Payload))
	copy(payload, c.Payload)
	var invalidationID [32]byte
	copy(invalidationID[:], c.InvalidationId[:])

	// the methodName needs to be the same as the 'name' above in the checkpointAbiJson
	// but other than that it's a constant that has no impact on the output. This is because
	// it gets encoded as a function name which we must then discard.
	abiEncodedCall, err := abi.Pack("checkpoint",
		gravityID,
		logicCallMethodName,
		transferAmounts,
		transferTokenContracts,
		feeAmounts,
		feeTokenContracts,
		common.HexToAddress(c.LogicContractAddress),
		payload,
		big.NewInt(int64(c.Timeout)),
		invalidationID,
		big.NewInt(int64(c.InvalidationNonce)),
	)

	// this should never happen outside of test since any case that could crash on encoding
	// should be filtered above.
	if err != nil {
		return nil, sdkerrors.Wrap(err, "packing checkpoint")
	}

	return crypto.Keccak256Hash(abiEncodedCall[4:]).Bytes(), nil
}
