package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
	//"sync"
	//"math/rand"
	"encoding/json"
	"io/ioutil"
	"strconv"
)

// Following constants can be changed in order to do performance testing.
const (
	toleranceDuration = 5 * time.Second
	pingDuration      = 100 * time.Millisecond
	testDuration      = 10 * time.Second
	MAXTOLERANCE      = 2
)

type logItem struct {
	Term  int
	Msg   interface{}
	MsgId int
}

type ServerPersist struct {
	LastApplied int
	VotedFor    int
	Log         []logItem
}

type Debug bool

//// For debugging capabilities..
func (d Debug) Println(a ...interface{}) {
	if d {
		fmt.Println(a...)
	}
}

const dbg Debug = true
const total_servers = 5

var proc []*exec.Cmd
var logFilePath, storeFilePath []string
var path, configFilePath, serverLogPath string

func initialize() {
	logFilePath = make([]string, total_servers)
	storeFilePath = make([]string, total_servers)
	path = os.Getenv("GOPATH") + "/bin/KVStore"
	serverLogPath = os.Getenv("GOPATH") + "/KVStore/server.log"
	
	configFilePath = os.Getenv("GOPATH") + "/KVStore/config.json"
	
	for i := 0; i < total_servers; i++ {
		logFilePath[i] = os.Getenv("GOPATH") + "/KVStore/" + strconv.Itoa(i+1) + ".log"
		storeFilePath[i] = os.Getenv("GOPATH") + "/KVStore/leveldblog" + strconv.Itoa(i+1)
	}
}

func checkLogs() bool {

	datatype := make([]*ServerPersist, total_servers)
	for i := 0; i < total_servers; i++ {
		file, e1 := ioutil.ReadFile(logFilePath[i])
		if e1 == nil {
			err := json.Unmarshal(file, &datatype[i])
			if err != nil {
				datatype[i] = &ServerPersist{
					Log:         []logItem{logItem{Term: -1, Msg: "Dummy"}},
					LastApplied: 0,
					VotedFor:    0}
			}
		}

	}
	min := 100000
	for i := 0; i < total_servers; i++ {
		min = func(x, y int) int {
			if x < y {
				return x
			}
			return y
		}(len(datatype[i].Log), min)
	}
	var flag bool
	for i := 0; i < min; i++ {
		flag = true
		a := datatype[0].Log[i]
		for j := 1; j < total_servers; j++ {
			if a == datatype[j].Log[i] {
				continue
			} else {
				flag = false
			}
		}
		if flag == false {
			break
		}
	}
	return flag
}

func Test_NoFault(t *testing.T) {

	proc = make([]*exec.Cmd, total_servers)

	initialize()


	for i := 0; i < total_servers; i++ {
		proc[i] = exec.Command(path, strconv.Itoa(i+1), configFilePath, logFilePath[i], storeFilePath[i])

		proc[i].Stdout = os.Stdout
		proc[i].Stderr = os.Stderr
	}
	for i := 0; i < total_servers; i++ {
		go func(i int) {
			er := proc[i].Run()
			if er != nil {
				fmt.Println("err", er)
			}
		}(i)
	}
	time.Sleep(5 * time.Second)
	for i := 0; i < total_servers; i++ {
		proc[i].Process.Kill()
	}
	cmd := exec.Command("killall -9 KVStore")
	cmd.Run()

	flag := checkLogs()
	if flag == true {
		t.Log("No fault test passed")
	} else {
		t.Error("Test failed, logs not equal")
	}
}

func Test_Stale(t *testing.T) {

	proc = make([]*exec.Cmd, total_servers)

	initialize()
	//left one server
	for i := 1; i < total_servers; i++ {
		proc[i] = exec.Command(path, strconv.Itoa(i+1), configFilePath, logFilePath[i], storeFilePath[i])

		proc[i].Stdout = os.Stdout
		proc[i].Stderr = os.Stderr
	}
	for i := 1; i < total_servers; i++ {
		go func(i int) {
			er := proc[i].Run()
			if er != nil {
				fmt.Println("err", er)
			}
		}(i)
	}
	time.Sleep(5 * time.Second)

	proc[0] = exec.Command(path, strconv.Itoa(1), configFilePath, logFilePath[0], storeFilePath[0])
	proc[0].Stdout = os.Stdout
	proc[0].Stderr = os.Stderr

	time.Sleep(10 * time.Second)

	for i := 0; i < total_servers; i++ {
		proc[i].Process.Kill()
	}
	cmd := exec.Command("killall -9 KVStore")
	cmd.Run()

	flag := checkLogs()
	if flag == true {
		t.Log("No fault test passed")
	} else {
		t.Error("Test failed, logs not equal")
	}
}
