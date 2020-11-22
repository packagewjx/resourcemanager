package utils

import (
	"reflect"
	"unsafe"
)

/*
#include <stdlib.h>
*/
import "C"

// 将Go语言的pid列表转换为C的pid_t类型的列表
// 使用完毕后务必free，否则将会引发内存泄漏
func MallocCPidList(pidList []int) unsafe.Pointer {
	sizeofPid := unsafe.Sizeof([1]C.pid_t{})
	buffer := C.malloc(C.ulong(uint64(sizeofPid) * uint64(len(pidList))))
	// 将malloc分配的pid_t数组转换为Go可用的Slice
	var cPidList []C.pid_t
	sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&cPidList))
	sliceHeader.Len = len(pidList)
	sliceHeader.Cap = len(pidList)
	sliceHeader.Data = uintptr(unsafe.Pointer(buffer))
	for idx, pid := range pidList {
		cPidList[idx] = C.pid_t(pid)
	}
	return buffer
}

func FreeCPointer(p unsafe.Pointer) {
	C.free(p)
}
