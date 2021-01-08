package pqos

/*
#cgo CFLAGS: -I/usr/local/include
#cgo LDFLAGS: -L/usr/local/lib -Wl,-rpath=/usr/local/lib -lpqos
#include <pqos.h>
#include <stdlib.h>

struct clos_scheme {
    int closNum; // CLOS号码
    pid_t *processList;
    unsigned int lenProcessList;
    unsigned int llc;
    unsigned int mbaThrottle;
};

int set_control_scheme(struct clos_scheme *schemes, int lenSchemes) {
    const struct pqos_cpuinfo *cpu;
    int ret = pqos_cap_get(NULL, &cpu);
    if (ret != PQOS_RETVAL_OK) {
        return ret;
    }

    unsigned int mbaIdCount, l3IdCount;
    unsigned int *mbaIds = pqos_cpu_get_mba_ids(cpu, &mbaIdCount);
    unsigned int *l3Ids = pqos_cpu_get_l3cat_ids(cpu, &l3IdCount);
    if (mbaIds == NULL || l3Ids == NULL) {
        return -1;
    }

    for (int i = 0; i < lenSchemes; i++) {
        // 设置L3分配
        if (schemes[i].llc != 0) {
            struct pqos_l3ca l3Ca = {
                    .class_id = schemes[i].closNum,
                    .u = {
                            .ways_mask = schemes[i].llc
                    }
            };
            for (int j = 0; j < l3IdCount; j++) {
                ret = pqos_l3ca_set(l3Ids[j], 1, &l3Ca);
                if (ret != PQOS_RETVAL_OK) {
                    return ret;
                }
            }
        }

        // 设置MBA分配
        if (schemes[i].mbaThrottle != 0) {
            struct pqos_mba mba = {
                    .class_id = schemes[i].closNum,
                    .ctrl = PQOS_MBA_ANY,
                    .mb_max = schemes[i].mbaThrottle
            };
            for (int j = 0; j < mbaIdCount; j++) {
                ret = pqos_mba_set(mbaIds[j], 1, &mba, NULL);
                if (ret != PQOS_RETVAL_OK) {
                    return ret;
                }
            }
        }

        // 设置进程绑定
        for (int j = 0; j < schemes->lenProcessList; j++) {
            // 这里忽略错误。由于可能会有很大量的PID设置，由一个进程设置错误会导致整个过程结束。比如设置过程中pid进程关闭了，重新设置
            // 又有可能新的进程关闭，可能就会多次重试。
            pqos_alloc_assoc_set_pid(schemes[i].processList[j], schemes[i].closNum);
        }
    }

    return 0;
}

*/
import "C"
import (
	"fmt"
	"github.com/packagewjx/resourcemanager/internal/utils"
	"os"
	"unsafe"
)

func init() {
	/**
	  int fd_log;
	  void (*callback_log)(void *context,
	                       const size_t size,
	                       const char *message);
	  void *context_log;
	  int verbose;
	  enum pqos_interface interface;
	*/
	pqosConfig := &C.struct_pqos_config{
		fd_log:       C.int(os.Stdout.Fd()),
		callback_log: nil,
		context_log:  nil,
		verbose:      0,
		_interface:   1,
	}
	res := C.pqos_init(pqosConfig)
	if res != 0 {
		panic("初始化pqos失败")
	}
}

func PqosFini() {
	C.pqos_fini()
}

type CLOSScheme struct {
	CLOSNum     int
	WayBit      int
	MemThrottle int
	Processes   []int
}

func SetCLOSScheme(schemes []*CLOSScheme) error {
	cSchemes := make([]C.struct_clos_scheme, len(schemes))
	pointerToFree := make([]unsafe.Pointer, 0)
	defer func() {
		for _, pointer := range pointerToFree {
			C.free(pointer)
		}
	}()
	for i, scheme := range schemes {
		list := utils.MallocCPidList(scheme.Processes)
		cSchemes[i] = C.struct_clos_scheme{
			closNum:        C.int(scheme.CLOSNum),
			processList:    (*C.pid_t)(list),
			lenProcessList: C.uint(len(scheme.Processes)),
			llc:            C.uint(scheme.WayBit),
			mbaThrottle:    C.uint(scheme.MemThrottle),
		}
		pointerToFree = append(pointerToFree, list)
	}

	res := int(C.set_control_scheme(&cSchemes[0], C.int(len(cSchemes))))
	if res != 0 {
		return fmt.Errorf("设置分配方案失败，返回码为%d", res)
	}

	return nil
}

func GetProcessCLOS(pid int) (int, error) {
	var classId C.uint
	res := C.pqos_alloc_assoc_get_pid(C.pid_t(pid), &classId)
	if res != 0 {
		return 0, fmt.Errorf("获取进程绑定关系错误，进程ID为%d", pid)
	}
	return int(classId), nil
}
