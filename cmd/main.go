package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {

	wg := &sync.WaitGroup{}

	arr := []int{1, 2, 3, 4, 5}
	copyArr := arr
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-time.After(2 * time.Second)
		fmt.Printf("arr: %v\n", arr)
		fmt.Printf("copyArr: %v\n", copyArr)
	}()
	arr = []int{}
	wg.Wait()
}
