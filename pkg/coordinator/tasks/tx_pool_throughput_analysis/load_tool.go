package txpoolcheck

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/noku-team/assertoor/pkg/coordinator/clients/execution"
	"github.com/noku-team/assertoor/pkg/coordinator/types"
	"github.com/sirupsen/logrus"
)

// LoadTool handles transaction execution logic
type LoadTool struct {
	task                 *Task
	logger               logrus.FieldLogger
	transactionGenerator *TxFactory
	peer                 *Peer
}

// NewLoadTool creates a new transaction executor
func NewLoadTool(task *Task, transactionGenerator *TxFactory, peer *Peer) *LoadTool {
	return &LoadTool{
		task:                 task,
		logger:               task.logger,
		transactionGenerator: transactionGenerator,
		peer:                 peer,
	}
}

// TxRequest represents a transaction request with its result
type TxRequest struct {
	tx  *ethtypes.Transaction
	err error
}

// TransactionReadResult represents the result of reading transaction messages
type TransactionReadResult struct {
	txs *eth.TransactionsPacket
	err error
}

// generateTransaction generates a transaction and sends it to the result channel
func (e *LoadTool) generateTransaction(ctx context.Context, idx int, txResultChan chan TxRequest, sentTxCount *int64, startTime time.Time, isFailed *bool) {
	// Generate and sign tx
	tx, err := e.transactionGenerator.GenerateTransaction(ctx)
	if err != nil {
		e.logger.Errorf("Failed to create transaction: %v", err)
		e.task.ctx.SetResult(types.TaskResultFailure)
		*isFailed = true
		txResultChan <- TxRequest{nil, err}
		return
	}

	// Increment counter and log progress
	atomic.AddInt64(sentTxCount, 1)
	currentCount := int(atomic.LoadInt64(sentTxCount))

	if currentCount%e.task.config.MeasureInterval == 0 {
		elapsed := time.Since(startTime)
		e.logger.Infof("Sent %d transactions in %.2fs", currentCount, elapsed.Seconds())
	}

	// Send to worker pool for processing
	txResultChan <- TxRequest{tx, nil}
}

// processTransactions processes transactions from the result channel
func (e *LoadTool) processTransactions(ctx context.Context, client *execution.Client, txResultChan chan TxRequest, txs *[]*ethtypes.Transaction, isFailed *bool, wg *sync.WaitGroup) {
	defer wg.Done()
	for txReq := range txResultChan {
		if txReq.err != nil || *isFailed {
			continue
		}

		// Send the transaction
		err := client.GetRPCClient().SendTransaction(ctx, txReq.tx)
		if err != nil {
			e.logger.WithField("client", client.GetName()).Errorf("Failed to send transaction: %v", err)
			e.task.ctx.SetResult(types.TaskResultFailure)
			*isFailed = true
			return
		}

		// Add to transaction lists
		*txs = append(*txs, txReq.tx)
	}
}

// sendTransactions sends transactions at the specified QPS rate
func (e *LoadTool) sendTransactions(ctx context.Context, client *execution.Client, qpsConfig QPSConfig, txs *[]*ethtypes.Transaction, sentTxCount *int64, isFailed *bool, sendingDone chan struct{}, startTime time.Time) {
	defer close(sendingDone)

	startExecTime := time.Now()
	endTime := startExecTime.Add(qpsConfig.Duration)

	// Calculate how many transactions to send in total
	totalTxToSend := int(qpsConfig.Duration.Seconds() * float64(qpsConfig.QPS))

	// Create a ticker to maintain consistent timing
	interval := time.Second / time.Duration(qpsConfig.QPS)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Channel for transaction results
	txResultChan := make(chan TxRequest, qpsConfig.QPS) // Buffer to prevent blocking

	// Start a worker pool to generate and send transactions
	const workerCount = 10 // Adjust based on performance needs
	var wg sync.WaitGroup

	// Launch workers to process transaction generation and sending
	for w := 0; w < workerCount; w++ {
		wg.Add(1)
		go e.processTransactions(ctx, client, txResultChan, txs, isFailed, &wg)
	}

	// Close the channel when we're done
	defer func() {
		close(txResultChan)
		wg.Wait() // Wait for all workers to finish
	}()

	// Main loop to schedule transactions at the correct rate
	for i := 0; i < totalTxToSend; i++ {
		if ctx.Err() != nil || *isFailed {
			return
		}

		// Wait for the next tick to maintain QPS
		<-ticker.C

		// Generate transaction in a separate goroutine to avoid blocking
		go e.generateTransaction(ctx, i, txResultChan, sentTxCount, startTime, isFailed)

		// Check if we've reached the duration limit
		if time.Now().After(endTime) {
			break
		}
	}

	execTime := time.Since(startExecTime)
	e.logger.Infof("Time to generate %d transactions at %d QPS: %v", int(atomic.LoadInt64(sentTxCount)), qpsConfig.QPS, execTime)
}

// ExecuteQPSLevel executes transactions at a specific QPS level
func (e *LoadTool) ExecuteQPSLevel(ctx context.Context, client *execution.Client, peer *Peer, qpsConfig QPSConfig) ([]*ethtypes.Transaction, int, time.Duration, float64, error) {
	e.logger.Infof("Starting test with QPS: %d for duration: %v", qpsConfig.QPS, qpsConfig.Duration)

	var txs []*ethtypes.Transaction
	startTime := time.Now()
	isFailed := false
	var sentTxCount int64 = 0 // Use int64 for atomic operations

	// Channel to signal when sending is complete
	sendingDone := make(chan struct{})

	// Start sending transactions at the specified QPS
	go e.sendTransactions(ctx, client, qpsConfig, &txs, &sentTxCount, &isFailed, sendingDone, startTime)

	// Monitor received transactions
	lastMeasureTime := time.Now()
	gotTx := 0

	if isFailed {
		return nil, 0, 0, 0, nil
	}

	// Wait for transactions to be processed or timeout
	processingTimeout := qpsConfig.Duration + 180*time.Second // Add 3 minutes buffer
	processingEndTime := time.Now().Add(processingTimeout)

	for {
		if isFailed {
			return nil, 0, 0, 0, nil
		}

		// Check if sending is complete and we've received all transactions
		select {
		case <-sendingDone:
			if gotTx >= int(atomic.LoadInt64(&sentTxCount)) {
				e.logger.Infof("All %d transactions processed for QPS %d", gotTx, qpsConfig.QPS)
				goto ProcessingComplete
			}
		default:
			// Continue processing
		}

		// Check if we've timed out
		if time.Now().After(processingEndTime) {
			e.logger.Warnf("Processing timeout after %v. Processed %d/%d transactions for QPS %d",
				processingTimeout, gotTx, int(atomic.LoadInt64(&sentTxCount)), qpsConfig.QPS)
			goto ProcessingComplete
		}

		// Add a timeout for reading transaction messages
		readChan := make(chan TransactionReadResult)

		// Read transaction messages in a separate goroutine
		go peer.ReadTransactionMessages(readChan)

		select {
		case result := <-readChan:
			if result.err != nil {
				e.logger.Errorf("Failed to read transaction messages: %v", result.err)
				e.task.ctx.SetResult(types.TaskResultFailure)
				return nil, 0, 0, 0, result.err
			}
			gotTx += len(*result.txs)
		case <-time.After(10 * time.Second):
			// Shorter timeout for more responsive updates
			continue
		}

		if gotTx%e.task.config.MeasureInterval == 0 && gotTx > 0 {
			txRate := float64(e.task.config.MeasureInterval) * float64(time.Second) / float64(time.Since(lastMeasureTime))
			e.logger.Infof("Got %d transactions", gotTx)
			e.logger.Infof("Tx/s: (%d txs processed): %.2f / s", e.task.config.MeasureInterval, txRate)
			lastMeasureTime = time.Now()
		}
	}

ProcessingComplete:
	totalTime := time.Since(startTime)
	txPerSecond := float64(gotTx) / totalTime.Seconds()

	e.logger.Infof("QPS %d: Processed %d transactions in %.2fs (%.2f tx/s)",
		qpsConfig.QPS, gotTx, totalTime.Seconds(), txPerSecond)

	return txs, gotTx, totalTime, txPerSecond, nil
}
