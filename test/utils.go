package test

import (
	"runtime"
	"strings"
)

func GetTestDataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return file[:strings.LastIndex(file, "/")] + "/data"
}

func GetTestConfigFile() string {
	_, file, _, _ := runtime.Caller(0)
	return file[:strings.LastIndex(file, "/")] + "/config.yaml"
}
