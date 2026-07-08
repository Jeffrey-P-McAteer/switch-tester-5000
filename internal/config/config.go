package config

import (
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Network NetworkConfig `toml:"network"`
	Test    TestConfig    `toml:"test"`
	Switch  SwitchConfig  `toml:"switch"`
}

type NetworkConfig struct {
	ListenPort int    `toml:"listen_port"`
	MirrorIP   string `toml:"mirror_ip"`
	MirrorPort int    `toml:"mirror_port"`
}

type TestConfig struct {
	StartBandwidthKbps   int     `toml:"start_bandwidth_kbps"`
	LossThresholdPercent float64 `toml:"loss_threshold_percent"`
	TestDurationSeconds  int     `toml:"test_duration_seconds"`
	PacketSizeBytes      int     `toml:"packet_size_bytes"`
}

type SwitchConfig struct {
	PortCount   int    `toml:"port_count"`
	MeasurePort int    `toml:"measure_port"`
	SwitchName  string `toml:"switch_name"`
}

func Default() *Config {
	return &Config{
		Network: NetworkConfig{
			ListenPort: 5001,
			MirrorIP:   "192.168.1.2",
			MirrorPort: 5000,
		},
		Test: TestConfig{
			StartBandwidthKbps:   500,
			LossThresholdPercent: 25.0,
			TestDurationSeconds:  3,
			PacketSizeBytes:      1024,
		},
		Switch: SwitchConfig{
			PortCount:   8,
			MeasurePort: 1,
			SwitchName:  "Switch",
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(cfg)
}
