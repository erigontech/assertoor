package types

import (
	"context"

	"github.com/noku-team/assertoor/pkg/coordinator/clients"
	"github.com/noku-team/assertoor/pkg/coordinator/db"
	"github.com/noku-team/assertoor/pkg/coordinator/logger"
	"github.com/noku-team/assertoor/pkg/coordinator/names"
	"github.com/noku-team/assertoor/pkg/coordinator/wallet"
	"github.com/sirupsen/logrus"
)

type Coordinator interface {
	Logger() logrus.FieldLogger
	LogReader() logger.LogReader
	Database() *db.Database
	ClientPool() *clients.ClientPool
	WalletManager() *wallet.Manager
	ValidatorNames() *names.ValidatorNames
	GlobalVariables() Variables
	TestRegistry() TestRegistry

	GetTestByRunID(runID uint64) Test
	GetTestQueue() []Test
	GetTestHistory(testID string, firstRunID uint64, offset uint64, limit uint64) ([]Test, int)
	ScheduleTest(descriptor TestDescriptor, configOverrides map[string]any, allowDuplicate bool) (TestRunner, error)
	DeleteTestRun(runID uint64) error
}

type TestRegistry interface {
	AddLocalTest(testConfig *TestConfig) (TestDescriptor, error)
	AddExternalTest(ctx context.Context, extTestConfig *ExternalTestConfig) (TestDescriptor, error)
	DeleteTest(testID string) error
	GetTestDescriptors() []TestDescriptor
}
