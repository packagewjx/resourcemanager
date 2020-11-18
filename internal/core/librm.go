package core

/*
#cgo LDFLAGS: -L/usr/local/lib -Wl,-rpath=/usr/local/lib -lresource_manager  -lpqos
#include <resource_manager.h>
#include <pqos.h>
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"reflect"
	"unsafe"
)

func LibInit() error {
	res := C.rm_init()
	if res != 0 {
		return fmt.Errorf("初始化librm失败，返回码为%d", res)
	}
	return nil
}

func LibFinalize() error {
	res := C.rm_finalize()
	if res != 0 {
		return fmt.Errorf("librm回收失败，返回码为%d", res)
	}
	return nil
}

type CLOSCapabilityInfo struct {
	numCatClos uint
	maxLLCWays uint
	minLLCWays uint
	numMbaClos uint
}

type ControlScheme struct {
	clos        uint
	pidList     []uint
	llc         uint // 缓存路数
	mbaThrottle uint // 内存控制阀门值
}

var capInfo *CLOSCapabilityInfo = nil

func GetCapabilityInfo() (*CLOSCapabilityInfo, error) {
	if capInfo == nil {
		buf := &C.struct_rm_capability_info{}
		res := C.rm_get_capability_info(buf)
		if res != 0 {
			return nil, fmt.Errorf("获取Capability错误，返回码为%d", res)
		}
		capInfo = &CLOSCapabilityInfo{
			numCatClos: uint(buf.numCatClos),
			maxLLCWays: uint(buf.maxLLCWays),
			minLLCWays: uint(buf.minLLCWays),
			numMbaClos: uint(buf.numMbaClos),
		}
	}

	return capInfo, nil
}

func SetControlScheme(schemes []*ControlScheme) error {
	sizeofPid := unsafe.Sizeof([1]C.pid_t{})
	cSchemes := make([]C.struct_rm_clos_scheme, len(schemes))
	pointersToFree := make([]unsafe.Pointer, len(schemes))
	for i, scheme := range schemes {
		buffer := C.malloc(C.ulong(uint64(sizeofPid) * uint64(len(scheme.pidList))))
		pointersToFree[i] = unsafe.Pointer(buffer)
		// 将malloc分配的pid_t数组转换为Go可用的Slice
		var pidList []C.pid_t
		sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&pidList))
		sliceHeader.Len = len(scheme.pidList)
		sliceHeader.Cap = len(scheme.pidList)
		sliceHeader.Data = uintptr(unsafe.Pointer(buffer))
		for idx, pid := range scheme.pidList {
			pidList[idx] = C.pid_t(pid)
		}
		cSchemes[i] = C.struct_rm_clos_scheme{
			closNum:        C.int(scheme.clos),
			processList:    &pidList[0],
			lenProcessList: C.uint(len(scheme.pidList)),
			llc:            C.uint(scheme.llc),
			mbaThrottle:    C.uint(scheme.mbaThrottle),
		}
	}

	res := int(C.rm_control_scheme_set(&cSchemes[0], C.int(len(cSchemes))))
	if res != 0 {
		return fmt.Errorf("设置分配方案失败，返回码为%d", res)
	}

	// 回收内存
	for _, pointer := range pointersToFree {
		C.free(pointer)
	}

	return nil
}

func GetProcessCLOS(pid uint) (uint, error) {
	var classId C.uint
	res := C.pqos_alloc_assoc_get_pid(C.pid_t(pid), &classId)
	if res != 0 {
		return 0, fmt.Errorf("获取进程绑定关系错误，进程ID为%d", pid)
	}
	return uint(classId), nil
}
