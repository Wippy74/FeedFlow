package closer

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloseUsesLIFOOrder(t *testing.T) {
	c := New()
	var order []string

	require.NoError(t, c.Add("first", func() error {
		order = append(order, "first")
		return nil
	}))
	require.NoError(t, c.Add("second", func() error {
		order = append(order, "second")
		return nil
	}))
	require.NoError(t, c.Add("third", func() error {
		order = append(order, "third")
		return nil
	}))

	require.NoError(t, c.Close())
	assert.Equal(t, []string{"third", "second", "first"}, order)
}

func TestCloseRunsAllFunctionsAndJoinsErrors(t *testing.T) {
	c := New()
	firstErr := errors.New("first failed")
	secondErr := errors.New("second failed")
	callCount := 0

	require.NoError(t, c.Add("first", func() error {
		callCount++
		return firstErr
	}))
	require.NoError(t, c.Add("second", func() error {
		callCount++
		return secondErr
	}))

	err := c.Close()
	require.Error(t, err)
	assert.ErrorIs(t, err, firstErr)
	assert.ErrorIs(t, err, secondErr)
	assert.Equal(t, 2, callCount)
}

func TestCloseIsIdempotent(t *testing.T) {
	c := New()
	callCount := 0
	require.NoError(t, c.Add("resource", func() error {
		callCount++
		return nil
	}))

	require.NoError(t, c.Close())
	require.NoError(t, c.Close())
	assert.Equal(t, 1, callCount)
	assert.ErrorIs(t, c.Add("late resource", func() error { return nil }), ErrClosed)
}
