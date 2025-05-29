package txpoolcheck

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/params"
	"github.com/noku-team/assertoor/pkg/coordinator/clients/execution"
	"github.com/noku-team/assertoor/pkg/coordinator/types"
	"github.com/noku-team/assertoor/pkg/coordinator/utils/sentry"
	"github.com/sirupsen/logrus"
)

// NetworkManager handles network connection and communication
type NetworkManager struct {
	task   *Task
	logger logrus.FieldLogger
}

// NewNetworkManager creates a new network manager
func NewNetworkManager(task *Task) *NetworkManager {
	return &NetworkManager{
		task:   task,
		logger: task.logger,
	}
}

// GetTcpConn establishes a TCP connection to the execution client
func (n *NetworkManager) GetTcpConn(ctx context.Context, client *execution.Client) (*sentry.Conn, error) {
	chainConfig := params.AllDevChainProtocolChanges

	head, err := client.GetRPCClient().GetLatestBlock(ctx)
	if err != nil {
		n.task.ctx.SetResult(types.TaskResultFailure)
		return nil, err
	}

	chainID, err := client.GetRPCClient().GetEthClient().ChainID(ctx)
	if err != nil {
		return nil, err
	}

	chainConfig.ChainID = chainID

	genesis, err := client.GetRPCClient().GetEthClient().BlockByNumber(ctx, new(big.Int).SetUint64(0))
	if err != nil {
		n.logger.Errorf("Failed to fetch genesis block: %v", err)
		n.task.ctx.SetResult(types.TaskResultFailure)
		return nil, err
	}

	conn, err := sentry.GetTcpConn(client)
	if err != nil {
		n.logger.Errorf("Failed to get TCP connection: %v", err)
		n.task.ctx.SetResult(types.TaskResultFailure)
		return nil, err
	}

	forkId := forkid.NewID(chainConfig, genesis, head.NumberU64(), head.Time())

	// handshake
	err = conn.Peer(chainConfig.ChainID, genesis.Hash(), head.Hash(), forkId, nil)
	if err != nil {
		return nil, err
	}

	n.logger.Infof("Connected to %s", client.GetName())

	return conn, nil
}

// ReadTransactionMessages reads transaction messages from the connection with a timeout
func (n *NetworkManager) ReadTransactionMessagesWithTimeout(ctx context.Context, conn *sentry.Conn, timeout int) (chan struct {
	txs *eth.TransactionsPacket
	err error
}, error) {
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

	return readChan, nil
}
