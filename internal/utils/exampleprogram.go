package utils

import (
	"fmt"
	"time"
)

func SequenceMemoryReader() {
	total := 0
	start := time.Now()
	mem := make([]int, 256*1024*1024)
	for time.Now().Sub(start) < 5*time.Second {
		for i := 1; i < len(mem); i++ {
			mem[i] += mem[i-1]
			total += mem[i]
		}
	}
	fmt.Println(total)
}

func RandomMemoryReader() {
	total := 0
	mem := make([]int, 1024*1024)
	for i := 0; i < len(mem); i++ {
		total += mem[i]
	}
	fmt.Println(total)
}
