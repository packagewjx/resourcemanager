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
	"github.com/packagewjx/resourcemanager/internal/utils"
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

var capInfo *CLOSCapabilityInfo = nil

func GetCapabilityInfo() (*CLOSCapabilityInfo, error) {
	if capInfo == nil {
		buf := &C.struct_rm_capability_info{}
		res := C.rm_get_capability_info(buf)
		if res != 0 {
			return nil, fmt.Errorf("获取Capability错误，返回码为%d", res)
		}
		capInfo = &CLOSCapabilityInfo{
			NumCatClos: uint(buf.numCatClos),
			MaxLLCWays: uint(buf.maxLLCWays),
			MinLLCWays: uint(buf.minLLCWays),
			NumMbaClos: uint(buf.numMbaClos),
		}
	}

	return capInfo, nil
}

func SetControlScheme(schemes []*CLOSScheme) error {
	cSchemes := make([]C.struct_rm_clos_scheme, len(schemes))
	pointersToFree := make([]unsafe.Pointer, len(schemes))
	for i, scheme := range schemes {
		var pidList []int
		for _, group := range scheme.ProcessGroups {
			pidList = append(pidList, group.Pid...)
		}
		list := utils.MallocCPidList(pidList)
		cSchemes[i] = C.struct_rm_clos_scheme{
			closNum:        C.int(scheme.CLOSNum),
			processList:    (*C.pid_t)(list),
			lenProcessList: C.uint(len(pidList)),
			llc:            C.uint(scheme.WayBit),
			mbaThrottle:    C.uint(scheme.MemThrottle),
		}
		C.free(list)
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
