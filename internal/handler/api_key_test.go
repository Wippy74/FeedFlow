package handler

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateAPIKey(t *testing.T) {
	first, err := generateAPIKey()
	require.NoError(t, err)
	second, err := generateAPIKey()
	require.NoError(t, err)

	assert.True(t, strings.HasPrefix(first, "na_"))
	assert.Len(t, first, 46)
	assert.NotEqual(t, first, second)
}
