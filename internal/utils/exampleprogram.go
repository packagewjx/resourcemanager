package utils

/*

#include <unistd.h>
#include <stdlib.h>
#include <sys/time.h>
#include <stdio.h>
#include <pthread.h>

const int bufSize = 128 * 1024 * 1024;

extern inline void sequenceAccess() {
    int *buf = malloc(sizeof(int) * bufSize);
    struct timeval start;
    gettimeofday(&start, NULL);
    struct timeval now;
    do {
        for (int i = 1; i < bufSize; i++) {
            buf[i] += buf[i - 1];
        }
        gettimeofday(&now, NULL);
    } while (now.tv_sec - start.tv_sec < 3);
    printf("%d\n", buf[bufSize - 1]);
    free(buf);
}

void *th(void *arg) {
    int *buf = arg;
    for (int i = 0; i < 10000000; i++) {
        int idx = (int) (((double) random() / RAND_MAX) * bufSize);
        buf[idx]++;
    }
    return NULL;
}

extern inline void concurrentRandomAccess() {
    const int count = 4;
    int *buf = malloc(sizeof(int) * bufSize);
    pthread_t threads[count];
    for (int i = 0; i < count; i++) {
        pthread_create(&threads[i], NULL, th, buf);
    }

    for (int i = 0; i < count; i++) {
        pthread_join(threads[i], NULL);
    }

    printf("%d\n", buf[0]);
    free(buf);
}

int forkRun(int num) {
    int pid = fork();
    if (pid == 0) {
        if (num == 1) {
            sequenceAccess();
        } else {
            concurrentRandomAccess();
        }
        exit(0);
    }
    return pid;
}

*/
import "C"
import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"
)

func ForkRunExample(functionNumber int) int {
	pid := int(C.forkRun(C.int(functionNumber)))

	for {
		file, err := ioutil.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err == nil {
			split := strings.Split(string(file), " ")
			if split[2] == "R" {
				return pid
			}
		}
		<-time.After(100 * time.Millisecond)
	}
}
