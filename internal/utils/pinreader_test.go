package utils

import (
	"github.com/stretchr/testify/assert"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRead(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	dirs := strings.Split(filepath.Dir(file), string(filepath.Separator))
	dirs = append(dirs[:len(dirs)-3], "test", "data", "ls.dat")
	file = strings.Join(dirs, string(filepath.Separator))
	reader, err := NewPinBinaryReader(file)
	assert.NoError(t, err)
	ch := reader.AsyncRead()
	for record := range ch {
		assert.NotZero(t, record.Tid)
		assert.NotZero(t, len(record.List))
	}
}
