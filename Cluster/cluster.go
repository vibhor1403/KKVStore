//Package cluster provides interface for creating a new cluster and sending and recieving messages through sockets.
// It also provides message structure to be passed.
package cluster

import (
	"encoding/json"
	"fmt"
	zmq "github.com/pebbe/zmq4"
	"io/ioutil"
	//"time"
)

const (
	BROADCAST = -1
	MAX       = 10
)

type Debug bool

//// For debugging capabilities..
func (d Debug) Println(a ...interface{}) {
	if d {
		fmt.Println(a...)
	}
}

const dbg Debug = false

// Envelope describes the message structure to be followed to communicate with other servers.
type Envelope struct {
	//SendTo specifies the pid of the recieving system. Setting it to -1, will broadcast the message to all peers.
	SendTo int

	// SendBy specifies the pid of the sender.
	SendBy int

	// MsgId is an id that globally and uniquely identifies the message, meant for duplicate detection at
	// higher levels. It is opaque to this package.
	MsgId int

	// the actual message.
	Msg interface{}

	//Term contains the current local value of term. Servers synchronize with each other using this value, by message passing.
	Term int

	// 	Type can hold the following values:
	//	REQUESTVOTE = 1
	//	HEARTBEAT   = 2
	//	GRANTVOTE   = 3
	//	MODIFY      = 4
	//	NOVOTE      = 5
	Type int

	// VoteTo is used for synchronizing in case any votes come out of order..
	VoteTo int
	//index of log entry immediately preceding new ones
	PrevLogIndex int
	//term of prevLogIndex entry
	PrevLogTerm int
	//leader’s commitIndex
	LeaderCommit int
}

// Server interface provides various methods for retriving information about the cluster.
type Server interface {
	// Pid is the Id of this server
	Pid() int

	// Peers contains array of other servers' ids in the same cluster
	Peers() []int

	// Outbox is the channel to use to send messages to other peers.
	Outbox() chan *Envelope

	// Inbox is the channel to receive messages from other peers.
	Inbox() chan *Envelope

	Stop() chan bool
}

// JSON object to read from and write to a file
type jsonobject struct {
	Object ObjectType
}

type ObjectType struct {
	Total   int
	Servers []ServerConfig
}

//ServerConfig is structure containing all the information needed about this server
type ServerConfig struct {
	// Pid of this server
	Mypid int
	// Url (ip:port) of this server
	Url string
	// Input channel for holding incoming data
	Input chan *Envelope
	// Output channel for sending data to other peers
	Output chan *Envelope
	// Array of peers
	Mypeers []int
	// Array of all sockets opened by this server (contains 4 outbound sockets)
	Sockets []*zmq.Socket
	// Stopping channel indicates the process of server shutdown.
	Stopping chan bool
}

func (sc ServerConfig) Pid() int {
	return sc.Mypid
}

func (sc ServerConfig) Peers() []int {
	return sc.Mypeers
}

func (sc ServerConfig) Inbox() chan *Envelope {
	return sc.Input
}

func (sc ServerConfig) Outbox() chan *Envelope {
	return sc.Output
}

func (sc ServerConfig) Stop() chan bool {
	return sc.Stopping
}

//func (sc ServerConfig) MsgRcvd() int {
//	return sc.N_msgRcvd
//}

//func (sc ServerConfig) MsgSent() int {
//	return sc.N_msgSent
//}

// mapping maps pid of the server to its url
var mapping map[int]string

// New function is the main function, which initializes all the parameters needed for server to function correctly. Further, it also
// starts routines to check for the channels concurrently..
func New(pid int, conf string) *ServerConfig {

	file, e := ioutil.ReadFile(conf)
	if e != nil {
		panic("Could not read file")
	}
	var jsontype jsonobject
	err := json.Unmarshal(file, &jsontype)
	if err != nil {
		panic("Wrong format of conf file")
	}

	// Inialization of mapping and server parameters.
	mapping = make(map[int]string)
	sc := &ServerConfig{
		Mypid:    pid,
		Url:      mapping[pid],
		Input:    make(chan *Envelope),
		Output:   make(chan *Envelope),
		Mypeers:  make([]int, jsontype.Object.Total-1),
		Sockets:  make([]*zmq.Socket, jsontype.Object.Total-1),
		Stopping: make(chan bool)}

	// Populates the peers of the servers, and opens the outbound ZMQ sockets for each of them.
	k := 0
	for i := 0; i < jsontype.Object.Total; i++ {
		mapping[jsontype.Object.Servers[i].Mypid] = jsontype.Object.Servers[i].Url

		if jsontype.Object.Servers[i].Mypid != pid {
			sc.Mypeers[k] = jsontype.Object.Servers[i].Mypid
			sc.Sockets[k], err = zmq.NewSocket(zmq.PUSH)

			if err != nil {
				panic(fmt.Sprintf("Unable to open socket for %v as %v", sc.Mypeers[k], err))
			}
			err = sc.Sockets[k].Connect("tcp://" + mapping[sc.Mypeers[k]])
			if err != nil {
				panic(fmt.Sprintf("Unable to connect socket for %v as %v", sc.Mypeers[k], err))
			}
			k++
		}
	}
	sc.Url = mapping[pid]

	// 	Starts two go routines each for input and output channel functionalities..
	//	go CheckInput(sc)
	dbg.Println("ye wale routine")
	go CheckOutput(sc)
	go Listen(sc)
	return sc
}

// CheckInput waits on input channel of the server and prints the data on standard output.
func CheckInput(sc *ServerConfig) {
	for {
		envelope, ok := <-sc.Input
		if !ok {
			panic("channels closed..")
		}
		dbg.Println("Received msg from %d to %d", envelope.SendBy, envelope.SendTo)
	}
}

// CheckOutput waits on output channel, and according to the type of message recieved on this channel sends the data to other peers.
// While sending the data, it checks whether the network link is down or not, as simulated in the peerPartition array.
func CheckOutput(sc *ServerConfig) {
	for {
		x, ok := <-sc.Output
		// If Output channel is not closed.
		if ok {
			// If BROADCAST message
			if x.SendTo == -1 {
				b, _ := json.Marshal(*x)
				//fmt.Println(b)
				for i := 0; i < len(sc.Mypeers); i++ {
					//dbg.Println("bhej raha hun broadcast", x)
					_, err := sc.Sockets[i].Send(string(b), 0)
					if err != nil {
						panic(fmt.Sprintf("Could not send message,%v,%v..%v", sc.Mypid, sc.Mypeers[i], err))
					}
				}
			} else {
				b, _ := json.Marshal(*x)
				//dbg.Println("bhej raha hun")
				_, err := sc.Sockets[findPid(sc, x.SendTo)].Send(string(b), 0)
				if err != nil {
					panic(fmt.Sprintf("Could not send 1message,%v,%v..%v", sc.Mypid, sc.Mypeers[findPid(sc, x.SendTo)], err))
				}
			}
			// If output channel is closed, then it closes all the outbound sockets of the current server.
		} else {
			for i := 0; i < len(sc.Mypeers); i++ {
				sc.Sockets[i].Close()
			}
			sc.Stopping <- true
			return
		}
	}
}

// findPid returns the index of server's peers array which correspond to the given pid. If not found, returns -1
func findPid(sc *ServerConfig, pid int) int {
	for i := 0; i < len(sc.Mypeers); i++ {
		if pid == sc.Mypeers[i] {
			return i
		}
	}
	return -1
}

// Listen method waits on recieving socket to gather data from the peers. Wait timeout is set to 2 seconds. If no data is available for
// more than 2 seconds, listen socket is closed, and correspondingly the input channel is also closed, which enables another routine
// waiting on input channel to be notified.
// This timeout can be set to -1 if we want that socket remains open for indefinite amount of time.
func Listen(sc *ServerConfig) {
	listenSocket, er := zmq.NewSocket(zmq.PULL)
	if er != nil {
		panic("Unable to open socket for listening")
	}
	defer listenSocket.Close()
	listenSocket.Bind("tcp://" + sc.Url)
	//listenSocket.SetRcvtimeo(5*time.Second)
	listenSocket.SetRcvtimeo(-1)
	for {
		//		if getState(sc) == CLOSEDSTATE {
		//			sc.Stopping <- true
		//			close(sc.Input)
		//			return
		//		}
		dbg.Println(sc.Mypeers)
		dbg.Println("sun ne ki koshish")
		msg, err := listenSocket.Recv(0)
		dbg.Println("mila")
		if err != nil {
			// If timeout happens and server is not issued a closed request, either the link is temporarily down, or all other peers are down.
			// In such case, start listening again.
			//			if getState(sc) != CLOSEDSTATE {
			//				go Listen(sc)
			//			} else {
			//				//close(sc.Input)
			//				sc.Stopping <- true
			//				close(sc.Input)
			//			}
			sc.Stopping <- true
			listenSocket.Close()
			dbg.Println("timeout in listen")
			return
		}

		message := new(Envelope)
		json.Unmarshal([]byte(msg), message)
		sc.Input <- message
		//		if getState(sc) == CLOSEDSTATE {
		//			sc.Stopping <- true
		//			close(sc.Input)
		//			return
		//		} else {
		//			sc.Input <- message
		//		}
	}
}
