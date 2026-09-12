package retry

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"time"
)

const (
	defaultMaxAttempts   = 8
	defaultBaseDelay     = time.Second
	defaultMaxDelay      = time.Hour
	defaultJitterPercent = 0.2
)

type Config struct {
	MaxAttempts   int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	JitterPercent float64

	RandomFloat64 func() float64
}

type Policy struct {
	maxAttempts   int
	baseDelay     time.Duration
	maxDelay      time.Duration
	jitterPercent float64

	randomFloat64 func() float64
}

type retryableError interface {
	Retryable() bool
}

type retryDelayError interface {
	RetryDelay() time.Duration
}

func DefaultPolicy() *Policy {
	policy, err := NewPolicy(Config{
		MaxAttempts:   defaultMaxAttempts,
		BaseDelay:     defaultBaseDelay,
		MaxDelay:      defaultMaxDelay,
		JitterPercent: defaultJitterPercent,
	})
	if err != nil {
		panic(err)
	}
	return policy
}

func NewPolicy(config Config) (*Policy, error) {
	if config.MaxAttempts <= 0 {
		return nil, fmt.Errorf("retry max attempts must be greater than zero")
	}
	if config.BaseDelay <= 0 {
		return nil, fmt.Errorf("retry base delay must be greater than zero")
	}
	if config.MaxDelay <= 0 {
		return nil, fmt.Errorf("retry max delay must be greater than zero")
	}
	if config.BaseDelay > config.MaxDelay {
		return nil, fmt.Errorf("retry base delay must not exceed max delay")
	}
	if config.JitterPercent < 0 || config.JitterPercent > 1 {
		return nil, fmt.Errorf("retry jitter percent must be between zero and one")
	}

	randomFloat64 := config.RandomFloat64
	if randomFloat64 == nil {
		randomFloat64 = rand.Float64
	}

	return &Policy{
		maxAttempts:   config.MaxAttempts,
		baseDelay:     config.BaseDelay,
		maxDelay:      config.MaxDelay,
		jitterPercent: config.JitterPercent,
		randomFloat64: randomFloat64,
	}, nil
}

func (p *Policy) NextDelay(ctx context.Context, attempt int, err error) (time.Duration, bool) {
	if p == nil || err == nil || attempt <= 0 {
		return 0, false
	}
	if ctx == nil || ctx.Err() != nil {
		return 0, false
	}
	if attempt >= p.maxAttempts || !IsRetryable(err) {
		return 0, false
	}

	delay := p.exponentialDelay(attempt)
	delay = p.withJitter(delay)

	if suggested := SuggestedDelay(err); suggested > delay {
		delay = suggested
	}

	return delay, true
}

func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	var classified retryableError
	if errors.As(err, &classified) {
		return classified.Retryable()
	}

	var networkErr net.Error
	return errors.As(err, &networkErr) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF)
}

func SuggestedDelay(err error) time.Duration {
	if err == nil {
		return 0
	}

	var provider retryDelayError
	if !errors.As(err, &provider) {
		return 0
	}

	delay := provider.RetryDelay()
	if delay < 0 {
		return 0
	}
	return delay
}

func (p *Policy) exponentialDelay(attempt int) time.Duration {
	delay := p.baseDelay
	for currentAttempt := 1; currentAttempt < attempt; currentAttempt++ {
		if delay >= p.maxDelay || delay > p.maxDelay/2 {
			return p.maxDelay
		}
		delay *= 2
	}

	if delay > p.maxDelay {
		return p.maxDelay
	}
	return delay
}

func (p *Policy) withJitter(delay time.Duration) time.Duration {
	if p.jitterPercent == 0 {
		return delay
	}

	randomValue := p.randomFloat64()
	if randomValue < 0 {
		randomValue = 0
	} else if randomValue > 1 {
		randomValue = 1
	}

	minimumFactor := 1 - p.jitterPercent
	factor := minimumFactor + 2*p.jitterPercent*randomValue
	jittered := float64(delay) * factor
	if jittered <= 0 {
		return 0
	}
	if jittered >= float64(p.maxDelay) {
		return p.maxDelay
	}
	return time.Duration(jittered)
}
