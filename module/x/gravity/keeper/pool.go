package keeper

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/common"

	"github.com/cosmos/cosmos-sdk/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/cosmos/gravity-bridge/module/x/gravity/types"
)

// CreateSendToEthereum
// - checks a counterpart denominator exists for the given voucher type
// - burns the voucher for transfer amount and fees
// - persists an OutgoingTx
// - adds the TX to the `available` TX pool via a second index
func (k Keeper) CreateSendToEthereum(ctx sdk.Context, sender sdk.AccAddress, counterpartReceiver string, amount sdk.Coin, fee sdk.Coin) (uint64, error) {
	totalAmount := amount.Add(fee)
	totalInVouchers := sdk.Coins{totalAmount}

	// If the coin is a gravity voucher, burn the coins. If not, check if there is a deployed ERC20 contract representing it.
	// If there is, lock the coins.

	isCosmosOriginated, tokenContract, err := k.DenomToERC20Lookup(ctx, totalAmount.Denom)
	if err != nil {
		return 0, err
	}

	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, totalInVouchers); err != nil {
		panic(err)
	}

	// If it is no a cosmos-originated asset we burn
	if !isCosmosOriginated {
		if err := k.bankKeeper.BurnCoins(ctx, types.ModuleName, totalInVouchers); err != nil {
			panic(err)
		}
	}

	// get next tx id from keeper
	nextID := k.IncrementLastSendToEthereumIDKey(ctx)

	// construct outgoing tx, as part of this process we represent
	// the token as an ERC20 token since it is preparing to go to ETH
	// rather than the denom that is the input to this function.

	// set the outgoing tx in the pool index
	k.SetUnbatchedSendToEthereum(ctx, &types.SendToEthereum{
		Id:                nextID,
		Sender:            sender.String(),
		EthereumRecipient: counterpartReceiver,
		Erc20Token:        types.NewSDKIntERC20Token(amount.Amount, tokenContract),
		Erc20Fee:          types.NewSDKIntERC20Token(fee.Amount, tokenContract),
	})

	return nextID, nil
}

// CancelSendToEthereum
// - checks that the provided tx actually exists
// - deletes the unbatched tx from the pool
// - issues the tokens back to the sender
func (k Keeper) CancelSendToEthereum(ctx sdk.Context, send *types.SendToEthereum) error {
	totalToRefund := send.Erc20Token.GravityCoin()
	totalToRefund.Amount = totalToRefund.Amount.Add(send.Erc20Fee.Amount)
	totalToRefundCoins := sdk.NewCoins(totalToRefund)
	isCosmosOriginated, _ := k.ERC20ToDenomLookup(ctx, send.Erc20Token.Contract)
	sender, _ := sdk.AccAddressFromBech32(send.Sender)

	// If it is not cosmos-originated the coins are minted
	if !isCosmosOriginated {
		if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, totalToRefundCoins); err != nil {
			return sdkerrors.Wrapf(err, "mint vouchers coins: %s", totalToRefundCoins)
		}
	}

	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sender, totalToRefundCoins); err != nil {
		return sdkerrors.Wrap(err, "sending coins from module account")
	}

	k.DeleteUnbatchedSendToEthereum(ctx, send.Id, send.Erc20Fee)
	return nil
}

func (k Keeper) SetUnbatchedSendToEthereum(ctx sdk.Context, ste *types.SendToEthereum) {
	ctx.KVStore(k.storeKey).Set(types.GetSendToEthereumKey(ste.Id, ste.Erc20Fee), k.cdc.MustMarshalBinaryBare(ste))
}

func (k Keeper) DeleteUnbatchedSendToEthereum(ctx sdk.Context, id uint64, fee types.ERC20Token) {
	ctx.KVStore(k.storeKey).Delete(types.GetSendToEthereumKey(id, fee))
}

func (k Keeper) IterateUnbatchedSendToEthereumsByContract(ctx sdk.Context, contract common.Address, cb func(*types.SendToEthereum) bool) {
	iter := prefix.NewStore(ctx.KVStore(k.storeKey), append([]byte{types.SendToEthereumKey}, contract.Bytes()...)).ReverseIterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var ste types.SendToEthereum
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &ste)
		if cb(&ste) {
			break
		}
	}
}

func (k Keeper) IterateUnbatchedSendToEthereums(ctx sdk.Context, cb func(*types.SendToEthereum) bool) {
	iter := prefix.NewStore(ctx.KVStore(k.storeKey), []byte{types.SendToEthereumKey}).ReverseIterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var ste types.SendToEthereum
		k.cdc.MustUnmarshalBinaryBare(iter.Value(), &ste)
		if cb(&ste) {
			break
		}
	}
}

func (k Keeper) GetUnbatchedSendToEthereums(ctx sdk.Context) []*types.SendToEthereum {
	var out []*types.SendToEthereum
	k.IterateUnbatchedSendToEthereums(ctx, func(ste *types.SendToEthereum) bool {
		out = append(out, ste)
		return false
	})
	return out
}

func (k Keeper) IncrementLastSendToEthereumIDKey(ctx sdk.Context) uint64 {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get([]byte{types.LastSendToEthereumIDKey})
	var id uint64 = 0
	if bz != nil {
		id = binary.BigEndian.Uint64(bz)
	}
	newId := id + 1
	bz = sdk.Uint64ToBigEndian(newId)
	store.Set([]byte{types.LastSendToEthereumIDKey}, bz)
	return newId
}
