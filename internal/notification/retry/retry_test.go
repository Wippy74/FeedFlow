package retry

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type classifiedError struct {
	retryable bool
	delay     time.Duration
}

func (e *classifiedError) Error() string {
	return "classified delivery error"
}

func (e *classifiedError) Retryable() bool {
	return e.retryable
}

func (e *classifiedError) RetryDelay() time.Duration {
	return e.delay
}

func TestPolicyExponentialBackoff(t *testing.T) {
	policy, err := NewPolicy(Config{
		MaxAttempts:   5,
		BaseDelay:     time.Second,
		MaxDelay:      5 * time.Second,
		JitterPercent: 0,
	})
	require.NoError(t, err)

	retryErr := &classifiedError{retryable: true}

	tests := []struct {
		attempt int
		delay   time.Duration
		retry   bool
	}{
		{attempt: 1, delay: time.Second, retry: true},
		{attempt: 2, delay: 2 * time.Second, retry: true},
		{attempt: 3, delay: 4 * time.Second, retry: true},
		{attempt: 4, delay: 5 * time.Second, retry: true},
		{attempt: 5, retry: false},
	}

	for _, test := range tests {
		delay, retry := policy.NextDelay(
			context.Background(),
			test.attempt,
			retryErr,
		)
		assert.Equal(t, test.retry, retry)
		assert.Equal(t, test.delay, delay)
	}
}

func TestPolicyRespectsSuggestedDelay(t *testing.T) {
	policy, err := NewPolicy(Config{
		MaxAttempts:   5,
		BaseDelay:     time.Second,
		MaxDelay:      time.Minute,
		JitterPercent: 0,
	})
	require.NoError(t, err)

	delay, retry := policy.NextDelay(
		context.Background(),
		1,
		&classifiedError{
			retryable: true,
			delay:     30 * time.Second,
		},
	)

	assert.True(t, retry)
	assert.Equal(t, 30*time.Second, delay)
}

func TestPolicyAppliesDeterministicJitter(t *testing.T) {
	policy, err := NewPolicy(Config{
		MaxAttempts:   5,
		BaseDelay:     10 * time.Second,
		MaxDelay:      time.Minute,
		JitterPercent: 0.2,
		RandomFloat64: func() float64 { return 1 },
	})
	require.NoError(t, err)

	delay, retry := policy.NextDelay(
		context.Background(),
		1,
		&classifiedError{retryable: true},
	)

	assert.True(t, retry)
	assert.Equal(t, 12*time.Second, delay)
}

func TestPolicyRejectsPermanentErrorAndCancelledContext(t *testing.T) {
	policy := DefaultPolicy()

	delay, retry := policy.NextDelay(
		context.Background(),
		1,
		&classifiedError{retryable: false},
	)
	assert.False(t, retry)
	assert.Zero(t, delay)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	delay, retry = policy.NextDelay(
		ctx,
		1,
		&classifiedError{retryable: true},
	)
	assert.False(t, retry)
	assert.Zero(t, delay)
}

func TestIsRetryableUsesWrappedClassifierAndNetworkErrors(t *testing.T) {
	assert.True(t, IsRetryable(fmt.Errorf(
		"send notification: %w",
		&classifiedError{retryable: true},
	)))
	assert.False(t, IsRetryable(&classifiedError{retryable: false}))

	networkErr := &net.DNSError{IsTimeout: true}
	assert.True(t, IsRetryable(networkErr))
	assert.False(t, IsRetryable(errors.New("invalid recipient")))
}

func TestNewPolicyValidation(t *testing.T) {
	tests := []Config{
		{},
		{MaxAttempts: 1, BaseDelay: 0, MaxDelay: time.Second},
		{MaxAttempts: 1, BaseDelay: 2 * time.Second, MaxDelay: time.Second},
		{
			MaxAttempts:   1,
			BaseDelay:     time.Second,
			MaxDelay:      time.Second,
			JitterPercent: 1.1,
		},
	}

	for _, config := range tests {
		_, err := NewPolicy(config)
		require.Error(t, err)
	}
}
