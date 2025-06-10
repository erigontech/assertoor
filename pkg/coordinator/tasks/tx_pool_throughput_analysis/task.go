package txpoolcheck

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/forkid"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/params"
	"github.com/noku-team/assertoor/pkg/coordinator/clients/execution"
	"github.com/noku-team/assertoor/pkg/coordinator/helper"
	"github.com/noku-team/assertoor/pkg/coordinator/types"
	"github.com/noku-team/assertoor/pkg/coordinator/utils/sentry"
	"github.com/noku-team/assertoor/pkg/coordinator/wallet"
	"github.com/sirupsen/logrus"
	vegeta "github.com/tsenart/vegeta/v12/lib"
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

	client := executionClients[rand.Intn(len(executionClients))]

	conn, err := t.getTcpConn(ctx, client)
	if err != nil {
		t.logger.Errorf("Failed to get wire eth TCP connection: %v", err)
		t.ctx.SetResult(types.TaskResultFailure)
		return nil
	}

	defer conn.Close()

	var txs []*ethtypes.Transaction
	var sentTxCount int
	var isFailed bool

	startTime := time.Now()

	// Create vegeta attacker
	attacker := NewTxAttacker(t, client, ctx)

	// Create a dummy targeter (required by vegeta interface but not used for transactions)
	targeter := func(tgt *vegeta.Target) error {
		*tgt = vegeta.Target{
			Method: "POST",
			URL:    "dummy://transaction",
		}
		return nil
	}

	// Create pacer for desired QPS over 1 second
	rate := vegeta.Rate{Freq: t.config.QPS, Per: time.Second}
	pacer := vegeta.ConstantPacer{Freq: rate.Freq, Per: rate.Per}

	// Attack for 1 second duration
	duration := time.Second
	results := attacker.Attack(targeter, pacer, duration, "tx_attack")

	// Process results
	done := make(chan bool)
	go func() {
		defer close(done)
		for result := range results {
			if result.Error != "" {
				t.logger.Errorf("Transaction error: %s", result.Error)
				t.ctx.SetResult(types.TaskResultFailure)
				isFailed = true
				return
			}

			sentTxCount++

			// Note: We can't easily get the actual tx object from vegeta results
			// so we'll generate them again for the broadcast to other clients
		}

		execTime := time.Since(startTime)
		t.logger.Infof("Time to send %d transactions: %v", sentTxCount, execTime)
	}()

	// Wait for transaction sending to complete
	<-done

	if isFailed {
		return nil
	}

	lastMeasureTime := time.Now()
	gotTx := 0

	for gotTx < t.config.QPS {
		if isFailed {
			return nil
		}

		// Add a timeout of 180 seconds for reading transaction messages
		readChan := make(chan struct {
			txs *eth.TransactionsPacket
			err error
		})

		go func() {
			txs, err := conn.ReadTransactionMessages()
			readChan <- struct {
				txs *eth.TransactionsPacket
				err error
			}{txs, err}
		}()

		select {
		case result := <-readChan:
			if result.err != nil {
				t.logger.Errorf("Failed to read transaction messages: %v", result.err)
				t.ctx.SetResult(types.TaskResultFailure)
				return nil
			}
			gotTx += len(*result.txs)
		case <-time.After(180 * time.Second):
			t.logger.Warnf("Timeout after 180 seconds while reading transaction messages. Re-sending transactions...")

			// Calculate how many transactions we're still missing
			missingTxCount := t.config.QPS - gotTx
			if missingTxCount <= 0 {
				break
			}

			// Re-send transactions to the original client
			for i := 0; i < missingTxCount && i < len(txs); i++ {
				err = client.GetRPCClient().SendTransaction(ctx, txs[i])
				if err != nil {
					t.logger.WithError(err).Errorf("Failed to re-send transaction message, error: %v", err)
					t.ctx.SetResult(types.TaskResultFailure)
					return nil
				}
			}

			t.logger.Infof("Re-sent %d transactions", missingTxCount)
			continue
		}

		if gotTx%t.config.MeasureInterval != 0 {
			continue
		}

		t.logger.Infof("Got %d transactions", gotTx)
		t.logger.Infof("Tx/s: (%d txs processed): %.2f / s \n", gotTx, float64(t.config.MeasureInterval)*float64(time.Second)/float64(time.Since(lastMeasureTime)))

		lastMeasureTime = time.Now()
	}

	totalTime := time.Since(startTime)
	t.logger.Infof("Total time for %d transactions: %.2fs", sentTxCount, totalTime.Seconds())

	// Generate transactions for broadcasting to other clients
	for range sentTxCount {
		tx, err := t.generateTransaction(ctx)
		if err != nil {
			t.logger.Errorf("Failed to generate transaction for broadcasting: %v", err)
			continue
		}
		txs = append(txs, tx)
	}

	// send to other clients, for speeding up tx mining
	for _, tx := range txs {
		for _, otherClient := range executionClients {
			if otherClient.GetName() == client.GetName() {
				continue
			}

			otherClient.GetRPCClient().SendTransaction(ctx, tx)
		}
	}

	outputs := map[string]interface{}{
		"total_time_mus": totalTime.Microseconds(),
		"qps":            t.config.QPS,
	}
	outputsJSON, _ := json.Marshal(outputs)
	t.logger.Infof("outputs_json: %s", string(outputsJSON))

	t.ctx.Outputs.SetVar("total_time_mus", totalTime.Milliseconds())
	t.ctx.SetResult(types.TaskResultSuccess)

	return nil
}

func (t *Task) getTcpConn(ctx context.Context, client *execution.Client) (*sentry.Conn, error) {
	chainConfig := params.AllDevChainProtocolChanges

	head, err := client.GetRPCClient().GetLatestBlock(ctx)
	if err != nil {
		t.ctx.SetResult(types.TaskResultFailure)
		return nil, err
	}

	chainID, err := client.GetRPCClient().GetEthClient().ChainID(ctx)
	if err != nil {
		return nil, err
	}

	chainConfig.ChainID = chainID

	genesis, err := client.GetRPCClient().GetEthClient().BlockByNumber(ctx, new(big.Int).SetUint64(0))
	if err != nil {
		t.logger.Errorf("Failed to fetch genesis block: %v", err)
		t.ctx.SetResult(types.TaskResultFailure)
		return nil, err
	}

	conn, err := sentry.GetTcpConn(client)
	if err != nil {
		t.logger.Errorf("Failed to get TCP connection: %v", err)
		t.ctx.SetResult(types.TaskResultFailure)
		return nil, err
	}

	forkId := forkid.NewID(chainConfig, genesis, head.NumberU64(), head.Time())

	// handshake
	err = conn.Peer(chainConfig.ChainID, genesis.Hash(), head.Hash(), forkId, nil)
	if err != nil {
		return nil, err
	}

	t.logger.Infof("Connected to %s", client.GetName())

	return conn, nil
}

func (t *Task) generateTransaction(ctx context.Context) (*ethtypes.Transaction, error) {
	tx, err := t.wallet.BuildTransaction(ctx, func(_ context.Context, nonce uint64, _ bind.SignerFn) (*ethtypes.Transaction, error) {
		addr := t.wallet.GetAddress()
		toAddr := &addr

		txAmount, _ := crand.Int(crand.Reader, big.NewInt(0).SetUint64(10*1e18))

		feeCap := &helper.BigInt{Value: *big.NewInt(100000000000)} // 100 Gwei
		tipCap := &helper.BigInt{Value: *big.NewInt(1000000000)}   // 1 Gwei

		var txObj = &ethtypes.DynamicFeeTx{
			ChainID:   t.ctx.Scheduler.GetServices().ClientPool().GetExecutionPool().GetBlockCache().GetChainID(),
			Nonce:     nonce,
			GasTipCap: &tipCap.Value,
			GasFeeCap: &feeCap.Value,
			Gas:       50000,
			To:        toAddr,
			Value:     txAmount,
			Data:      []byte{},
		}

		return ethtypes.NewTx(txObj), nil
	})

	if err != nil {
		return nil, err
	}

	return tx, nil
}

// TxAttacker implements a custom vegeta attacker for transaction sending
type TxAttacker struct {
	task   *Task
	client *execution.Client
	ctx    context.Context
}

// NewTxAttacker creates a new transaction attacker
func NewTxAttacker(task *Task, client *execution.Client, ctx context.Context) *TxAttacker {
	return &TxAttacker{
		task:   task,
		client: client,
		ctx:    ctx,
	}
}

// Attack implements the vegeta attacker interface for transaction sending
func (a *TxAttacker) Attack(targeter vegeta.Targeter, pacer vegeta.Pacer, duration time.Duration, name string) <-chan *vegeta.Result {
	results := make(chan *vegeta.Result)

	go func() {
		defer close(results)

		var (
			began  = time.Now()
			target vegeta.Target
			err    error
			count  int
		)

		for {
			elapsed := time.Since(began)
			if elapsed >= duration {
				break
			}

			// Get next target (not used for transactions but required by interface)
			if err = targeter(&target); err != nil {
				results <- &vegeta.Result{
					Error: err.Error(),
				}
				continue
			}

			// Wait for the pacer
			if hit, ok := pacer.Pace(elapsed, uint64(count)); !ok {
				break
			} else if hit > 0 {
				time.Sleep(hit)
			}

			// Generate and send transaction
			before := time.Now()
			tx, err := a.task.generateTransaction(a.ctx)
			if err != nil {
				results <- &vegeta.Result{
					Error:   fmt.Sprintf("Failed to generate transaction: %v", err),
					Latency: time.Since(before),
				}
				count++
				continue
			}

			err = a.client.GetRPCClient().SendTransaction(a.ctx, tx)
			latency := time.Since(before)

			result := &vegeta.Result{
				Latency: latency,
			}

			if err != nil {
				result.Error = fmt.Sprintf("Failed to send transaction: %v", err)
				result.Code = 500 // Use HTTP-like error codes
			} else {
				result.Code = 200 // Success
				result.BytesOut = uint64(tx.Size())
			}

			results <- result
			count++

			// Log progress
			if count%a.task.config.MeasureInterval == 0 {
				a.task.logger.Infof("Sent %d transactions in %.2fs", count, elapsed.Seconds())
			}
		}
	}()

	return results
}
