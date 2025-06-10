package txpoolcheck

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/noku-team/assertoor/pkg/coordinator/clients/execution"
	"github.com/noku-team/assertoor/pkg/coordinator/types"
	"github.com/noku-team/assertoor/pkg/coordinator/wallet"
	"github.com/sirupsen/logrus"
)

var (
	TaskName       = "tx_pool_throughput_analysis"
	TaskDescriptor = &types.TaskDescriptor{
		Name:        TaskName,
		Description: "Checks the throughput of transactions in the Ethereum TxPool",
		Config:      DefaultConfig(),
		NewTask:     NewTask,
	}
)

type Task struct {
	ctx     *types.TaskContext
	options *types.TaskOptions
	wallet  *wallet.Wallet
	config  Config
	logger  logrus.FieldLogger
}

func NewTask(ctx *types.TaskContext, options *types.TaskOptions) (types.Task, error) {
	return &Task{
		ctx:     ctx,
		options: options,
		logger:  ctx.Logger.GetLogger(),
	}, nil
}

func (t *Task) Config() interface{} {
	return t.config
}

func (t *Task) Timeout() time.Duration {
	return t.options.Timeout.Duration
}

func (t *Task) LoadConfig() error {
	config := DefaultConfig()

	if t.options.Config != nil {
		if err := t.options.Config.Unmarshal(&config); err != nil {
			return fmt.Errorf("error parsing task config for %v: %w", TaskName, err)
		}
	}

	err := t.ctx.Vars.ConsumeVars(&config, t.options.ConfigVars)
	if err != nil {
		return err
	}

	if err := config.Validate(); err != nil {
		return err
	}

	privKey, _ := crypto.HexToECDSA(config.PrivateKey)
	t.wallet, err = t.ctx.Scheduler.GetServices().WalletManager().GetWalletByPrivkey(privKey)
	if err != nil {
		return fmt.Errorf("cannot initialize wallet: %w", err)
	}

	t.config = config
	return nil
}

func (t *Task) Execute(ctx context.Context) error {
	err := t.wallet.AwaitReady(ctx)
	if err != nil {
		return fmt.Errorf("cannot load wallet state: %w", err)
	}

	executionClients := t.ctx.Scheduler.GetServices().ClientPool().GetExecutionPool().GetReadyEndpoints(true)

	if len(executionClients) == 0 {
		t.logger.Errorf("No execution clients available")
		t.ctx.SetResult(types.TaskResultFailure)
		return nil
	}

	var client *execution.Client = executionClients[rand.Intn(len(executionClients))]

	// Initialize components
	transactionGenerator := NewTransactionGenerator(t)
	networkManager := NewNetworkManager(t)
	txExecutor := NewTxExecutor(t, transactionGenerator, networkManager)
	resultsManager := NewResultsManager(t)

	// Establish TCP connection
	conn, err := networkManager.GetTcpConn(ctx, client)
	if err != nil {
		t.logger.Errorf("Failed to get wire eth TCP connection: %v", err)
		t.ctx.SetResult(types.TaskResultFailure)
		return nil
	}
	defer conn.Close()

	// Create a table to store results for each QPS level
	var results []QPSResult
	var allTxs []*ethtypes.Transaction

	// Iterate through each QPS level
	for _, qpsConfig := range t.config.QPSLevels {
		txs, gotTx, totalTime, txPerSecond, err := txExecutor.ExecuteQPSLevel(ctx, client, conn, qpsConfig)
		if err != nil {
			t.logger.Errorf("Failed to execute QPS level %d: %v", qpsConfig.QPS, err)
			t.ctx.SetResult(types.TaskResultFailure)
			return nil
		}

		// Add to all transactions
		allTxs = append(allTxs, txs...)

		// Add result to our table
		results = append(results, QPSResult{
			QPS:            qpsConfig.QPS,
			TxProcessed:    gotTx,
			ProcessingTime: totalTime,
			TxPerSecond:    txPerSecond,
		})
	}

	// Display results table
	resultsManager.DisplayResults(results)

	// Send to other clients, for speeding up tx mining
	for _, tx := range allTxs {
		for _, otherClient := range executionClients {
			if otherClient.GetName() == client.GetName() {
				continue
			}

			otherClient.GetRPCClient().SendTransaction(ctx, tx)
		}
	}

	// Create JSON output with all results
	resultsManager.CreateOutputs(results)

	t.ctx.SetResult(types.TaskResultSuccess)
	return nil
}
