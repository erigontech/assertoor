package txpoolcheck

import (
	"context"
	crand "crypto/rand"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/noku-team/assertoor/pkg/coordinator/helper"
)

// TransactionGenerator handles transaction generation logic
type TransactionGenerator struct {
	task *Task
}

// NewTransactionGenerator creates a new transaction generator
func NewTransactionGenerator(task *Task) *TransactionGenerator {
	return &TransactionGenerator{
		task: task,
	}
}

// buildDynamicFeeTx constructs a dynamic fee transaction for the wallet
func (g *TransactionGenerator) buildDynamicFeeTx(_ context.Context, nonce uint64, _ bind.SignerFn) (*ethtypes.Transaction, error) {
	addr := g.task.wallet.GetAddress()
	toAddr := &addr

	txAmount, _ := crand.Int(crand.Reader, big.NewInt(0).SetUint64(10*1e18))

	feeCap := &helper.BigInt{Value: *big.NewInt(100000000000)} // 100 Gwei
	tipCap := &helper.BigInt{Value: *big.NewInt(1000000000)}   // 1 Gwei

	var txObj ethtypes.TxData

	txObj = &ethtypes.DynamicFeeTx{
		ChainID:   g.task.ctx.Scheduler.GetServices().ClientPool().GetExecutionPool().GetBlockCache().GetChainID(),
		Nonce:     nonce,
		GasTipCap: &tipCap.Value,
		GasFeeCap: &feeCap.Value,
		Gas:       50000,
		To:        toAddr,
		Value:     txAmount,
		Data:      []byte{},
	}

	return ethtypes.NewTx(txObj), nil
}

// GenerateTransaction creates a new transaction
func (g *TransactionGenerator) GenerateTransaction(ctx context.Context) (*ethtypes.Transaction, error) {
	tx, err := g.task.wallet.BuildTransaction(ctx, g.buildDynamicFeeTx)

	if err != nil {
		return nil, err
	}

	return tx, nil
}
