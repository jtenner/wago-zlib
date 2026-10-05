package wagozlib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	DefaultMaxInputBytes           uint32 = 8 << 20
	DefaultMaxOutputBytes          uint32 = 32 << 20
	DefaultMaxConcurrentOperations uint32 = 2

	hardMaxInputBytes           uint32 = 64 << 20
	hardMaxOutputBytes          uint32 = 64 << 20
	hardMaxConcurrentOperations uint32 = 8
)

// Config is the strict, reviewed plugin configuration. Zero values select the
// finite defaults. The hard maxima keep even reviewed configurations within a
// known per-runtime ceiling.
type Config struct {
	MaxInputBytes           uint32 `json:"maxInputBytes,omitempty"`
	MaxOutputBytes          uint32 `json:"maxOutputBytes,omitempty"`
	MaxConcurrentOperations uint32 `json:"maxConcurrentOperations,omitempty"`
}

func (c Config) resolved() (Config, error) {
	if c.MaxInputBytes == 0 {
		c.MaxInputBytes = DefaultMaxInputBytes
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = DefaultMaxOutputBytes
	}
	if c.MaxConcurrentOperations == 0 {
		c.MaxConcurrentOperations = DefaultMaxConcurrentOperations
	}
	if c.MaxInputBytes > hardMaxInputBytes {
		return Config{}, fmt.Errorf("maxInputBytes %d exceeds hard maximum %d", c.MaxInputBytes, hardMaxInputBytes)
	}
	if c.MaxOutputBytes > hardMaxOutputBytes {
		return Config{}, fmt.Errorf("maxOutputBytes %d exceeds hard maximum %d", c.MaxOutputBytes, hardMaxOutputBytes)
	}
	if c.MaxConcurrentOperations > hardMaxConcurrentOperations {
		return Config{}, fmt.Errorf("maxConcurrentOperations %d exceeds hard maximum %d", c.MaxConcurrentOperations, hardMaxConcurrentOperations)
	}
	return c, nil
}

func validateConfig(raw json.RawMessage) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return err
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing JSON value")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("configuration must be an object")
	}
	for name, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s must not be null", name)
		}
	}
	if _, present := fields["maxInputBytes"]; present && cfg.MaxInputBytes == 0 {
		return fmt.Errorf("maxInputBytes must be positive")
	}
	if _, present := fields["maxOutputBytes"]; present && cfg.MaxOutputBytes == 0 {
		return fmt.Errorf("maxOutputBytes must be positive")
	}
	if _, present := fields["maxConcurrentOperations"]; present && cfg.MaxConcurrentOperations == 0 {
		return fmt.Errorf("maxConcurrentOperations must be positive")
	}
	_, err := cfg.resolved()
	return err
}
