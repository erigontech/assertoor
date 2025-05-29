package txpoolcheck

import (
	"time"
)

// QPSConfig defines a QPS value and its duration
type QPSConfig struct {
	QPS      int           `yaml:"qps" json:"qps"`
	Duration time.Duration `yaml:"duration" json:"duration"`
}

type Config struct {
	PrivateKey string `yaml:"privateKey" json:"privateKey"`

	QPSLevels            []QPSConfig `yaml:"qpsLevels" json:"qpsLevels"`
	MeasureInterval      int         `yaml:"measureInterval" json:"measureInterval"`
	SecondsBeforeRunning int         `yaml:"secondsBeforeRunning" json:"secondsBeforeRunning"`
}

func DefaultConfig() Config {
	return Config{
		QPSLevels: []QPSConfig{
			{QPS: 10, Duration: 30 * time.Second},
			{QPS: 100, Duration: 30 * time.Second},
			{QPS: 1000, Duration: 30 * time.Second},
		},
		MeasureInterval:      100,
		SecondsBeforeRunning: 0,
	}
}

func (c *Config) Validate() error {
	return nil
}
