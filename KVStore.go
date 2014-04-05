package main

import (
	"github.com/vibhor1403/KVStore/Raft"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

func main() {

	a, _ := strconv.Atoi(os.Args[1])
	server := Raft.New(a /* config file */, os.Args[2], os.Args[3], os.Args[4])

	wg := new(sync.WaitGroup)
	wg.Add(1)

	go func() {
		ClientTimer := time.NewTimer(time.Second)
		for {
			select {
			case <-ClientTimer.C:
				if server.State() == Raft.LEADER {
					key := strconv.Itoa(rand.Intn(100000))
					value := strconv.Itoa(rand.Intn(100000))
					server.Inbox() <- "set " + key + " " + value
				}
				ClientTimer = time.NewTimer(100 * time.Millisecond)
			}
		}
	}()

	wg.Wait()
}
