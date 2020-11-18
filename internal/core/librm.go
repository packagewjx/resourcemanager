package core

/*
#cgo LDFLAGS: -L/usr/local/lib -Wl,-rpath=/usr/local/lib -lresource_manager  -lpqos
#include <resource_manager.h>
*/
import "C"

func LibInit() int {
	return int(C.rm_init())
}

func LibFinalize() int {
	return int(C.rm_finalize())
}
