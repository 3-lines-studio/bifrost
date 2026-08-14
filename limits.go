package bifrost

import "fmt"

const (
	defaultMaxPropsBytes    = 1 << 20
	defaultMaxHeadBytes     = 256 << 10
	defaultMaxFrameBytes    = 1 << 20
	defaultMaxMarkdownBytes = 8 << 20
	maxWireFrameBytes       = 16 << 20
)

// Limits bounds buffered, attacker-influenced render data. The streamed total
// body size is not capped.
type Limits struct {
	MaxPropsBytes    int
	MaxHeadBytes     int
	MaxFrameBytes    int
	MaxMarkdownBytes int
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits.MaxPropsBytes < 0 || limits.MaxHeadBytes < 0 || limits.MaxFrameBytes < 0 || limits.MaxMarkdownBytes < 0 {
		return Limits{}, fmt.Errorf("bifrost: limits must not be negative")
	}
	if limits.MaxHeadBytes > maxWireFrameBytes {
		return Limits{}, fmt.Errorf("bifrost: MaxHeadBytes cannot exceed %d", maxWireFrameBytes)
	}
	if limits.MaxFrameBytes > maxWireFrameBytes {
		return Limits{}, fmt.Errorf("bifrost: MaxFrameBytes cannot exceed %d", maxWireFrameBytes)
	}
	if limits.MaxPropsBytes == 0 {
		limits.MaxPropsBytes = defaultMaxPropsBytes
	}
	if limits.MaxHeadBytes == 0 {
		limits.MaxHeadBytes = defaultMaxHeadBytes
	}
	if limits.MaxFrameBytes == 0 {
		limits.MaxFrameBytes = defaultMaxFrameBytes
	}
	if limits.MaxMarkdownBytes == 0 {
		limits.MaxMarkdownBytes = defaultMaxMarkdownBytes
	}
	return limits, nil
}
