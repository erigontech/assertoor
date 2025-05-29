package txpoolcheck

import (
	"encoding/json"
	"time"

	"github.com/sirupsen/logrus"
)

// QPSResult represents the result of a QPS test
type QPSResult struct {
	QPS            int
	TxProcessed    int
	ProcessingTime time.Duration
	TxPerSecond    float64
}

// ResultsManager handles results processing and reporting
type ResultsManager struct {
	task   *Task
	logger logrus.FieldLogger
}

// NewResultsManager creates a new results manager
func NewResultsManager(task *Task) *ResultsManager {
	return &ResultsManager{
		task:   task,
		logger: task.logger,
	}
}

// DisplayResults displays the results table
func (r *ResultsManager) DisplayResults(results []QPSResult) {
	r.logger.Infof("=== Throughput Analysis Results ===")
	r.logger.Infof("%-10s | %-15s | %-15s | %-15s", "QPS", "Tx Processed", "Time (s)", "Tx/s")
	r.logger.Infof("%-10s | %-15s | %-15s | %-15s", "----------", "---------------", "---------------", "---------------")

	for _, result := range results {
		r.logger.Infof("%-10d | %-15d | %-15.2f | %-15.2f",
			result.QPS, result.TxProcessed, result.ProcessingTime.Seconds(), result.TxPerSecond)
	}
}

// CreateOutputs creates JSON outputs for the task
func (r *ResultsManager) CreateOutputs(results []QPSResult) {
	outputResults := make([]map[string]interface{}, len(results))
	for i, result := range results {
		outputResults[i] = map[string]interface{}{
			"qps":             result.QPS,
			"tx_processed":    result.TxProcessed,
			"processing_time": result.ProcessingTime.Milliseconds(),
			"tx_per_second":   result.TxPerSecond,
		}
	}

	outputs := map[string]interface{}{
		"results": outputResults,
	}
	outputsJSON, _ := json.Marshal(outputs)
	r.logger.Infof("outputs_json: %s", string(outputsJSON))

	r.task.ctx.Outputs.SetVar("throughput_results", outputResults)
}
