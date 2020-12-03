package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestNumBits(t *testing.T) {
	assert.Equal(t, 1, NumBits(1))
	assert.Equal(t, 1, NumBits(0x10000))
	assert.Equal(t, 0, NumBits(0))
	assert.Equal(t, 16, NumBits(0xFFFF))
	assert.Equal(t, 1, NumBits(2))
}
