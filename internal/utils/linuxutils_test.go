package utils

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
	"unsafe"
)

func TestMallocCPidList(t *testing.T) {
	testArray := []int{1, 2, 3, 4}
	list := MallocCPidList(testArray)
	var slice []int32
	header := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	header.Cap = 4
	header.Len = 4
	header.Data = uintptr(list)
	for i := 0; i < len(testArray); i++ {
		assert.Equal(t, testArray[i], int(slice[i]))
	}
	FreeCPointer(list)
}
