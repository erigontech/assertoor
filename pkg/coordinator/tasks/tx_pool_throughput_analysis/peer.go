package txpoolcheck

import (
	"context"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/core/forkid"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/ethereum/go-ethereum/params"
	"github.com/noku-team/assertoor/pkg/coordinator/clients/execution"
	"github.com/noku-team/assertoor/pkg/coordinator/types"
	"github.com/noku-team/assertoor/pkg/coordinator/utils/sentry"
	"github.com/sirupsen/logrus"
)

// A P2P transaction announcement message
type P2PTxMessage struct {
	txs *eth.TransactionsPacket // an array of transactions
	err error
}

// Peer connects to an execution client (a bockchain node) on the p2p network (i.e., the peer of the node)
type Peer struct {
	task   *Task
	logger logrus.FieldLogger
	node   *execution.Client
	conn   *sentry.Conn
}

// NewPeer creates a new peer
func NewPeer(task *Task) *Peer {
	return &Peer{
		task:   task, // Task used to report errors and results, please remove in the future
		logger: task.logger,
	}
}

// Connect establishes a TCP connection to the execution client
func (s *Peer) Connect(client *execution.Client) error {
	s.node = client

	chainConfig := params.AllDevChainProtocolChanges

	head, err := client.GetRPCClient().GetLatestBlock(context.Background())
	if err != nil {
		s.task.ctx.SetResult(types.TaskResultFailure)
		return err
	}

	chainID, err := client.GetRPCClient().GetEthClient().ChainID(context.Background())
	if err != nil {
		return err
	}

	chainConfig.ChainID = chainID

	genesis, err := client.GetRPCClient().GetEthClient().BlockByNumber(context.Background(), new(big.Int).SetUint64(0))
	if err != nil {
		s.logger.Errorf("Failed to fetch genesis block: %v", err)
		s.task.ctx.SetResult(types.TaskResultFailure)
		return err
	}

	conn, err := sentry.GetTcpConn(client)
	if err != nil {
		s.logger.Errorf("Failed to get TCP connection: %v", err)
		s.task.ctx.SetResult(types.TaskResultFailure)
		return err
	}

	forkId := forkid.NewID(chainConfig, genesis, head.NumberU64(), head.Time())

	// handshake
	err = conn.Peer(chainConfig.ChainID, genesis.Hash(), head.Hash(), forkId, nil)
	if err != nil {
		return err
	}

	s.logger.Infof("Connected to %s", client.GetName())

	// Store the connection
	s.conn = conn

	return nil
}

func (s *Peer) Close() {
	if s.conn == nil {
		s.logger.Warnf("No active connection to close for %s", s.node.GetName())
		return
	}

	s.logger.Infof("Closing connection to %s", s.node.GetName())
	if err := s.conn.Close(); err != nil {
		s.logger.Errorf("Failed to close connection: %v", err)
	}
	s.conn = nil
}

func (s *Peer) ReadTransactionMessages(readChan chan TransactionReadResult) {
	// Check if connection is nil
	if s.conn == nil {
		s.logger.Errorf("Peer has no active connection, cannot read transaction messages")
		s.task.ctx.SetResult(types.TaskResultFailure)
		return
	}

	txs, err := s.conn.ReadTransactionMessages()
	readChan <- TransactionReadResult{txs, err}
}

func (s *Peer) ReadTransactionMessagesInNewChannel() (chan P2PTxMessage, error) {
	// Check if connection is nil
	if s.conn == nil {
		s.logger.Errorf("Peer has no active connection, cannot read transaction messages")
		s.task.ctx.SetResult(types.TaskResultFailure)
		return nil, errors.New("peer connection is nil")
	}

	readChan := make(chan P2PTxMessage)

	go func() {
		txs, err := s.conn.ReadTransactionMessages()
		readChan <- P2PTxMessage{txs, err}
	}()

	return readChan, nil
}
