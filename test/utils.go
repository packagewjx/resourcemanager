package test

import (
	"runtime"
	"strings"
)

func GetDataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return file[:strings.LastIndex(file, "/")] + "/data"
}
