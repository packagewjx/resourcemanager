package pin

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"syscall"
)

func mkTempFifo() (string, error) {
	var name string
	for {
		name = fmt.Sprintf("pin-%x.fifo", rand.Int())
		_, err := os.Stat(name)
		if os.IsNotExist(err) {
			break
		}
	}

	err := syscall.Mkfifo(name, 0600)
	return name, err
}

type logWriter struct {
	prefix string
	logger *log.Logger
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	l.logger.Printf("%s %s", l.prefix, string(p))
	return len(p), nil
}
