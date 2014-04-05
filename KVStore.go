package main

import (
	"github.com/vibhor1403/KVStore/Raft"
	"fmt"
	"os"
	"time"
	"strconv"
	"sync"
)

func main() {

	// parse argument flags and get this server's id into myid
	//mypid := flag.Int("pid", 0, "Pid of my own system")
	//mylog := flag.String("log", "", "Log File")
	//flag.Parse()
	//var input string
	fmt.Println("chalu to hua1")
	//fmt.Scanln(&input)
	a,_ := strconv.Atoi(os.Args[1])
	server := Raft.New(a /* config file */, os.Args[2], os.Args[3], os.Args[4])
	
	fmt.Println(server)
//	

	wg := new(sync.WaitGroup)
	wg.Add(1)

	
	fmt.Println("chalu to hua")
	go func() {
		fmt.Println("in goroutine")
		//time.Sleep(2*time.Second)
		ClientTimer := time.NewTimer(time.Second)
		for {
			select {
			case <- ClientTimer.C:
				fmt.Println("in goroutineccccccccccccccccccccccccccccccccccccccccccc", a)
				if server.State() == Raft.LEADER {
					fmt.Println("inside LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL")
					server.Inbox() <- "get"
				}
				ClientTimer = time.NewTimer(100*time.Millisecond)
				//fmt.Println(server.Log())
			}
		}
	}()
	
	// wait for keystroke.
/*	fmt.Println("in goroutine")
		for {
			time.Sleep(time.Second)
		}
	fmt.Scanln(&input)
	*/
	wg.Wait()
}

