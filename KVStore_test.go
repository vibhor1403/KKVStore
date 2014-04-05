package main

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
	//"sync"
	//"math/rand"
	"strconv"
	"io/ioutil"
	"encoding/json"
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
	Log      []logItem 
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
var logFilePath []string

func Test_NoFault(t *testing.T) {

	proc = make([]*exec.Cmd, total_servers)
	logFilePath = make([]string, total_servers)
	path := os.Getenv("GOPATH") + "/bin/KVStore"
	configFilePath := os.Getenv("GOPATH") + "/src/github.com/vibhor1403/KVStore/config.json"
	storeFilePath := os.Getenv("HOME") + "/Desktop/leveldblog.db"

	fmt.Println("GOPATH=", path)
	for i := 0; i < total_servers; i++ {
		logFilePath[i] = os.Getenv("HOME") + "/Desktop/" + strconv.Itoa(i+1) + ".log"
		proc[i] = exec.Command(path, strconv.Itoa(i+1), configFilePath, logFilePath[i], storeFilePath)
		fmt.Println(proc[i])

		proc[i].Stdout = os.Stdout
		proc[i].Stderr = os.Stdout
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

	datatype := make([]*ServerPersist, total_servers)
	for i := 0; i < total_servers; i++ {
		file, e1 := ioutil.ReadFile(logFilePath[i])
		if e1 == nil {
			//ioutil.WriteFile(logFile, []byte(""), os.ModeExclusive)
			err := json.Unmarshal(file, &datatype[i])
			if err != nil {
				datatype[i] = &ServerPersist{
					Log: []logItem{logItem{Term: -1, Msg: "Dummy"}}}
			}
		}

	}
	min := 100000 
	for i:=0; i<total_servers; i++ {
		min = func(x, y int) int {
				if x < y {
					return x
				}
				return y
			}(len(datatype[i].Log), min)
	}
	fmt.Println("minimum", min)
	var flag bool
	for i:= 0 ; i<min; i++ {
		flag = true
		a := datatype[0].Log[i]
		for j:=1; j<total_servers; j++ {
			if a == datatype[j].Log[i] {
				continue
			}else {
				flag = false
			}
		}
		if flag == false {
			break
		} 
	}
	fmt.Println("flag", flag)
}
