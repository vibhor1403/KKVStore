//Package Raft implements the Leader election process of Raft consensus algorithm.
package Raft

import (
	"encoding/json"
	"fmt"
	cluster "github.com/vibhor1403/KVStore/Cluster"
	"io/ioutil"
	"math/rand"
	"os"
	"reflect"
	"sync"
	"time"
	//"strconv"
	///"math"
)

// Various constants for configuring server...
const (
	NONE              = 0
	BROADCAST         = -1
	MAX               = 10
	FOLLOWER          = 1
	CANDIDATE         = 2
	LEADER            = 3
	CLOSEDSTATE       = 4
	REQUESTVOTE       = 1
	HEARTBEAT         = 2
	GRANTVOTE         = 3
	MODIFY            = 4
	NOVOTE            = 5
	APPEND            = 6
	APPENDSUCCESS     = 7
	APPENDFAIL        = 8
	timeoutDuration   = 200 * time.Millisecond
	heartbeatinterval = 50 * time.Millisecond
	recieveTO         = 2 * time.Second
)

type Debug bool

//// For debugging capabilities..
func (d Debug) Println(a ...interface{}) {
	if d {
		fmt.Println(a...)
	}
}

const dbg Debug = false
const dbg1 Debug = false
const dbg2 Debug = false
const dbg3 Debug = true

// Server interface provides various methods for retriving information about the cluster.
type Server interface {
	// Pid is the Id of this server
	Pid() int

	// Peers contains array of other servers' ids in the same cluster
	Peers() []int

	// Outbox is the channel to use to send messages to other peers.
	Outbox() chan interface{}

	// Inbox is the channel to receive messages from other peers.
	Inbox() chan interface{}

	// Leader tells the present leader.
	Leader() int

	// Error channel helps to control the closing of servers. It shuts down all the sockets and channels.
	Error() chan bool

	// State returns the current state of the server. It can have following values:
	//	FOLLOWER    = 1
	//	CANDIDATE   = 2
	//	LEADER      = 3
	//	CLOSEDSTATE	= 4
	State() int

	// Server Stopped channel synchronizes the closing of server. It will have a value, only when this server is completely closed,	
	// ie all the sockets, channels and goroutines are properly shut down.
	ServerStopped() chan bool

	Log() []logItem
}

type logItem struct {
	Term  int
	Msg   interface{}
	MsgId int
}

type RequestMessage struct {
	Msg       interface{}
	index     int
	term      int
	appendAck int
}

type ServerPersist struct {
	Log      []logItem 
}

type jsonobject struct {
	Object ObjectType
}

type ObjectType struct {
	Total   int
	Servers []ServerConfig
}

//ServerConfig is structure containing all the information needed about this server
type ServerConfig struct {
	basicCluster *cluster.ServerConfig
	ErrorChannel chan bool
	// Stopped channel indicates proper closing of server.
	Stopped chan bool

	// Array of peers

	mutex sync.RWMutex
	// state has values: 1 - follower, 2 - candidate, 3 - leader, 4 - closed
	state int
	// stores pid, can have value 0, indicating no vote, as pid's start from 1
	votedFor int
	// stores the current local value of term.
	term int
	// stores the pid of leader. At stable state, all servers will have same value in this field.
	leader int
	// At what point to break the election process in idle situation.
	electionTODuration time.Duration
	// Rate of sending keep alive messages.
	heartbeatDuration time.Duration
	// majority holds the maximum fault tolerance capability.
	majority int
	// peerPartition contains the array with information of network connectivity.
	log            []logItem
	Input          chan interface{}
	Output         chan interface{}
	readyForClient bool
	appendsrcvd    int
	//******************************************
	//VOLATILE STORAGE

	//index of highest log entry known to be committed (initialized to 0, increases monotonically)
	commitIndex int
	//index of highest log entry applied to state machine (initialized to 0, increases monotonically)
	lastApplied int

	//for each server, index of the next log entry to send to that server (initialized to leader last log index + 1)
	nextIndex []int
	//for each server, index of highest log entry known to be replicated on server (initialized to 0, increases monotonically)
	matchIndex []int
	//************************************************
	// committed index will be currentLogIndex-1 as of now. (Not accepting more than 1 client request at a time.)
	currentLogIndex int

	ready chan bool

	msgId    int
	inFlight int

	NoOfPeers int
	msgSend   []int
}

func (sc *ServerConfig) Pid() int {
	return sc.basicCluster.Mypid
}

func (sc *ServerConfig) Peers() []int {
	return sc.basicCluster.Mypeers
}

func (sc *ServerConfig) Inbox() chan interface{} {
	return sc.Input
}

func (sc *ServerConfig) Outbox() chan interface{} {
	return sc.Output
}

func (sc *ServerConfig) Error() chan bool {
	return sc.ErrorChannel
}

func (sc *ServerConfig) Leader() int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.leader
}

func (sc *ServerConfig) State() int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.state
}

func (sc *ServerConfig) ServerStopped() chan bool {
	return sc.Stopped
}

func (sc *ServerConfig) Log() []logItem {
	return sc.log
}

// mapping maps pid of the server to its url
var mapping map[int]string

// New function is the main function, which initializes all the parameters needed for server to function correctly. Further, it also
// starts routines to check for the channels concurrently..
func New(pid int, conf string, logFile string) Server {

	file, e := ioutil.ReadFile(conf)
	if e != nil {
		panic("Could not read file")
	}
	var jsontype jsonobject
	err := json.Unmarshal(file, &jsontype)
	if err != nil {
		panic("Wrong format of conf file")
	}

	file1, e1 := ioutil.ReadFile(logFile)
	var datatype *ServerPersist
	if e1 == nil {
		err := json.Unmarshal(file1, &datatype)
		if err != nil {
			datatype = &ServerPersist{
				Log:      []logItem{logItem{Term: -1, Msg: "Dummy"}} }
		}
	}

	// Inialization of mapping and server parameters.
	mapping = make(map[int]string)
	sc := &ServerConfig{
		basicCluster:       cluster.New(pid, conf),
		ErrorChannel:       make(chan bool),
		Stopped:            make(chan bool),
		state:              FOLLOWER,
		votedFor:           NONE,
		term:               datatype.Log[len(datatype.Log)-1].Term,
		leader:             NONE,
		electionTODuration: timeoutDuration,
		heartbeatDuration:  heartbeatinterval,
		majority:           (jsontype.Object.Total/2 + 1),
		log:                datatype.Log,
		Input:              make(chan interface{}),
		Output:             make(chan interface{}),
		readyForClient:     false,
		commitIndex:        len(datatype.Log) - 1,
		lastApplied:        0,
		nextIndex:          make([]int, jsontype.Object.Total-1),
		matchIndex:         make([]int, jsontype.Object.Total-1),
		currentLogIndex:    len(datatype.Log) - 1,
		appendsrcvd:        0,
		ready:              make(chan bool),
		NoOfPeers:          jsontype.Object.Total - 1,
		// to be written on file (i believe)
		msgId:    datatype.Log[len(datatype.Log)-1].MsgId + 1,
		inFlight: 0,
		msgSend:  make([]int, jsontype.Object.Total-1)}

	//Initialize nextIndex
	for i := 0; i < jsontype.Object.Total-1; i++ {
		sc.nextIndex[i] = sc.commitIndex + 1
	}

	//Initialize match Index
	for i := 0; i < jsontype.Object.Total-1; i++ {
		sc.matchIndex[i] = 0
	}

	for i := 0; i < jsontype.Object.Total-1; i++ {
		sc.msgSend[i] = 0
	}

	dbg3.Println(sc.log,sc.nextIndex,"term",sc.term,"msgid",sc.msgId,"commitindex",sc.commitIndex)

	dbg.Println("go routine started")
	go CheckState(sc)
	go clientInbox(sc)
	go persistData(sc, logFile)
	go printIndex(sc)
	return sc
}

func printIndex (sc *ServerConfig) {
	timer := time.NewTimer(time.Second)
	for {
		<-timer.C
		dbg3.Println("commitIndex", sc.commitIndex)
		timer = time.NewTimer(time.Second)
	}
}

func permanentBackup (sc *ServerConfig) {
	timer := time.NewTimer(time.Second)
	for {
		<-timer.C
		
		timer = time.NewTimer(time.Second)
	}
}


func persistData(sc *ServerConfig, logFile string) {
	timer := time.NewTimer(time.Second)
	for {
		<-timer.C
		persistSC := &ServerPersist{
			Log:      sc.log }

		//dbg3.Println(sc.log[0:5])
		
		//dbg2.Println("save time commit index", sc.commitIndex)
		//persistSC.Log = persistSC.Log[0:sc.commitIndex+1]
		
		//f, er := os.Create("/home/vibhor/Desktop/"+logFile+".txt")
		//fmt.Println("about to print", persistSC)
		data, err := json.Marshal(persistSC)
		if err != nil {
			fmt.Println("error")
		}
		er := ioutil.WriteFile(logFile, data, os.ModeExclusive)
		if er != nil {
			fmt.Println("error", er)
		}
		//defer f.Close()
		//_, er = f.Write(data)
		//if er != nil {
		//	fmt.Println("Error", er)
		//}
		timer = time.NewTimer(time.Second)
	}
}

// CheckState checks constantly for the current state of the server, and accordingly calls the appropriate loop.
// In closed state, it waits for all the go routines to properly shut-down and then signals the public channel.
func CheckState(sc *ServerConfig) {

	for {
		sc.mutex.Lock()
		state := sc.state
		sc.mutex.Unlock()
		//dbg.Println("state", sc.state)

		switch state {
		case FOLLOWER:
			followerLoop(sc)
		case CANDIDATE:
			candidateLoop(sc)
		case LEADER:
			leaderLoop(sc)
		case CLOSEDSTATE:
			<-sc.basicCluster.Stop()
			<-sc.basicCluster.Stop()
			//dbg.Println(sc.basicCluster.Mypid, "stopped")
			sc.Stopped <- true
			return
		}
	}
}

// Gets a write lock and sets the value of the current leader.
func setLeader(sc *ServerConfig, pid int) {
	sc.mutex.Lock()
	sc.leader = pid
	sc.mutex.Unlock()
}

// Gets a read lock and reads the value of the current leader.
func getLeader(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.leader
}

// Gets a write lock and sets the value of the current term.
func setTerm(sc *ServerConfig, term int) {
	sc.mutex.Lock()
	sc.term = term
	sc.mutex.Unlock()
}

// Gets a write lock and sets the value of the peer to whom the vote has been given.
func setVotedFor(sc *ServerConfig, pid int) {
	sc.mutex.Lock()
	sc.votedFor = pid
	sc.mutex.Unlock()
}

// Gets a read lock and reads the value of the current term.
func getTerm(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.term
}

// Gets a read lock and reads the value of the peer to whom this server voted..
func getVotedFor(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.votedFor
}

// Gets a read lock and reads the value of the current state.
func getState(sc *ServerConfig) int {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.state
}

// Gets a write lock and sets the value of the current state.
func setState(sc *ServerConfig, state int) {
	sc.mutex.Lock()
	sc.state = state
	sc.mutex.Unlock()
}
func setReady(sc *ServerConfig, state bool) {
	sc.mutex.Lock()
	sc.readyForClient = state
	sc.mutex.Unlock()
}
func getReady(sc *ServerConfig) bool {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	return sc.readyForClient
}

// Waits for a random time between given duration and duration*2, and sends the current time on
// the returned channel.
func RandomTimer(duration time.Duration) <-chan time.Time {
	rand := rand.New(rand.NewSource(time.Now().UnixNano()))
	d, delta := duration, duration
	if delta > 0 {
		d += time.Duration(rand.Int63n(int64(delta)))
	}
	return time.After(d)
}

// follower loop realizes the logic of FOLLOWER as given in Raft consensus algorithm.
func followerLoop(sc *ServerConfig) {
	dbg.Println("In follower...")
	timeChan := RandomTimer(sc.electionTODuration)

	//dbg.Println(sc.basicCluster.Mypid, "state", state)
	for getState(sc) == FOLLOWER {
		//wait for inbox channel
		//dbg.Println(sc.basicCluster.Mypid, "waiting at channel", sc.term, sc.state)
		select {
		case envelope := <-sc.basicCluster.Input:
			//dbg1.Println("recieve, infollower", sc.basicCluster.Mypid, envelope)
			// if a lower term message is recieved, send a modify message...
			if envelope.Term < getTerm(sc) {
				dbg.Println(sc.basicCluster.Mypid, "send, infollower1", envelope.SendBy, sc.term, "MODIFY")
				sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: MODIFY}
				// if heartbeat recieved, reset timer, update term if needed
			} else if envelope.Term > getTerm(sc) && envelope.Type == REQUESTVOTE {
				setTerm(sc, envelope.Term)
				setVotedFor(sc, envelope.SendBy)
				dbg.Println(sc.basicCluster.Mypid, "send, infollower2", envelope.SendBy, getTerm(sc), "GRANTVOTE")
				sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				// If heartbeat is recieved, sets the current term and updates leader.
			} else if envelope.Type == HEARTBEAT {
				setTerm(sc, envelope.Term)
				setLeader(sc, envelope.SendBy)
				// if request for vote recieved, take decision of granting vote or not and sent vote on channel and reset timer.
			} else if envelope.Type == REQUESTVOTE && len(sc.log) <= envelope.PrevLogIndex + 1 { 
				if sc.log[len(sc.log)-1].Term == envelope.PrevLogTerm {
					setTerm(sc, envelope.Term)
					if getVotedFor(sc) == NONE {
						setVotedFor(sc, envelope.SendBy)
						dbg1.Println(sc.basicCluster.Mypid, "send, infollower3", envelope.SendBy, getTerm(sc), "GRANTVOTE")
						sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
					} else {
						dbg.Println(sc.basicCluster.Mypid, "send, infollower4", envelope.SendBy, getTerm(sc), "NOVOTE")
						sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: NOVOTE, VoteTo: sc.votedFor}
					}
				} else {
					dbg.Println(sc.basicCluster.Mypid, "send, infollower4", envelope.SendBy, getTerm(sc), "NOVOTE")
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: NOVOTE, VoteTo: sc.votedFor}
				}
				//Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm 
			} else if envelope.Type == APPEND {
				dbg3.Println("APPEND recieved", envelope)
				dbg3.Println(sc.log, "currindex", sc.currentLogIndex)
				
				maped := envelope.Msg.(map[string]interface{})
				msg := maped["Msg"]
				dbg1.Println("type:", reflect.TypeOf(msg))
				term := maped["Term"].(float64)
				//dbg1.Println("type:", reflect.TypeOf(term))
				msgId := int(maped["MsgId"].(float64))
				
				if len(sc.log)-1 < envelope.PrevLogIndex || sc.log[envelope.PrevLogIndex].Term != envelope.PrevLogTerm {
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, MsgId: msgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDFAIL}
					dbg3.Println("APPENFAIL1 sent", &cluster.Envelope{SendTo: envelope.SendBy, MsgId: msgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDFAIL})
				} else if sc.currentLogIndex == envelope.PrevLogIndex+1 && sc.log[sc.currentLogIndex].Term != envelope.Term {
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, MsgId: msgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDFAIL}
					dbg3.Println("APPENFAIL2 sent", &cluster.Envelope{SendTo: envelope.SendBy, MsgId: msgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDFAIL})
					sc.log = sc.log[0 : sc.currentLogIndex]
					sc.currentLogIndex -= 1
				} else {
					//dbg1.Println("type:", reflect.TypeOf(envelope.Msg))
					

					if int(msgId) > sc.log[len(sc.log)-1].MsgId {
	
						sc.log = append(sc.log, logItem{Term: int(term), Msg: msg, MsgId: int(msgId)})
	
						sc.currentLogIndex += 1
						dbg3.Println("log index", sc.currentLogIndex)
						dbg1.Println("envelope", envelope)
						sc.msgId = envelope.MsgId+1
						sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, MsgId: envelope.MsgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDSUCCESS}
						if envelope.LeaderCommit > sc.commitIndex {
							dbg2.Println("settin commit index", envelope.LeaderCommit, sc.commitIndex)
							sc.commitIndex = func(x, y int) int {
								if x < y {
									return x
								}
								return y
							}(envelope.LeaderCommit, sc.currentLogIndex)
						} 
						dbg3.Println("APPENDSUCCESS", &cluster.Envelope{SendTo: envelope.SendBy, MsgId: envelope.MsgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDSUCCESS})
						dbg1.Println(&cluster.Envelope{SendTo: envelope.SendBy, MsgId: envelope.MsgId, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: APPENDSUCCESS})
					}
				}
			}
			// reset the timer.
			timeChan = RandomTimer(sc.electionTODuration)
			// If timer expires, promote itself to candidate and start a new election.
		case <-timeChan:
			dbg.Println("timer expired")
			sc.mutex.Lock()
			sc.state = CANDIDATE
			sc.votedFor = NONE
			sc.mutex.Unlock()
			dbg.Println(sc.basicCluster.Mypid, "become candidate", sc.term, sc.state)
			// If error channel has recieved a value, close the server.
		case <-sc.ErrorChannel:
			closeServer(sc)
		}
	}

}

// closeServer Stops the server and is called whenever a value id recieved on error channel. It simulates the breakdown of the server.
// closes all the channels and set the state to CLOSEDSTATE.
func closeServer(sc *ServerConfig) {
	setState(sc, CLOSEDSTATE)
	close(sc.basicCluster.Output)
	close(sc.ErrorChannel)
	//close(sc.Input)
}

// candidateLoop realizes the logic of CANDIDATE as given in Raft consensus algorithm.
func candidateLoop(sc *ServerConfig) {
	//	dbg.Println(sc.basicCluster.Mypid, "entered candidate")
	// increment term
	sc.mutex.Lock()
	sc.term++
	sc.mutex.Unlock()

	//	dbg.Println(sc.basicCluster.Mypid, "term", sc.term)
	// vote for self
	setVotedFor(sc, sc.basicCluster.Mypid)
	totalVotesRecieved := 1
	//reset timer
	timeChan := RandomTimer(sc.electionTODuration)
	// send request for voting on all peers
	dbg.Println("send, incandidate", -1, sc.term)
	sc.basicCluster.Output <- &cluster.Envelope{SendTo: -1, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: REQUESTVOTE, PrevLogIndex: len(sc.log)-1, PrevLogTerm: sc.log[len(sc.log)-1].Term}

	for getState(sc) == CANDIDATE {

		select {
		// wait for inbox channel
		case envelope := <-sc.basicCluster.Input:
			dbg.Println("recieve incandidate", sc.basicCluster.Mypid, envelope)
			// if a lower term message is recieved, send a modify message...
			if envelope.Term < getTerm(sc) {
				dbg.Println(sc.basicCluster.Mypid, "send, incandidate1", envelope.SendBy, getTerm(sc), "MODIFY")
				sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: MODIFY}
				// if recieved message term is greater than my term, end election, become follower, return
			} else if envelope.Type == HEARTBEAT || envelope.Type == APPEND {
				setTerm(sc, envelope.Term)
				setState(sc, FOLLOWER)
				setVotedFor(sc, NONE)
				setLeader(sc, envelope.SendBy)
				dbg.Println(sc.basicCluster.Mypid, "became follower")
				return
			} else if envelope.Term > getTerm(sc) {
				setTerm(sc, envelope.Term)
				// if bigger term peer requests for vote, give him vote without thinking, as your vote to yourself is stale
				if envelope.Type == REQUESTVOTE {
					setVotedFor(sc, envelope.SendBy)
					dbg.Println(sc.basicCluster.Mypid, "send, incandidate2", envelope.SendBy, getTerm(sc), "GRANTVOTE")
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				}
				setState(sc, FOLLOWER)
				return
				// if vote is granted, add to total votes
			} else if envelope.Type == GRANTVOTE && envelope.VoteTo == sc.basicCluster.Mypid {
				totalVotesRecieved++
				dbg.Println("Votes", totalVotesRecieved)
			}

			// if majority votes recieved, become leader, return
			if totalVotesRecieved >= sc.majority {
				setState(sc, LEADER)
				setVotedFor(sc, NONE)
				setLeader(sc, sc.basicCluster.Mypid)
				dbg.Println(sc.basicCluster.Mypid, "became leader")
				//setReady(sc, true)
				sc.ready <- true
				return
			}
			// if timeout
		case <-timeChan:
			// become follower, end election.
			dbg.Println(sc.basicCluster.Mypid, "became follower again")
			setVotedFor(sc, NONE)
			setState(sc, FOLLOWER)
			// If error channel has some value.
		case <-sc.ErrorChannel:
			closeServer(sc)
		}
	}
}

func clientInbox(sc *ServerConfig) {
	dbg.Println("in client inbox")
	for {
		<-sc.ready
		//if getReady(sc) == true {
		dbg.Println("in client inbox waiting")
		//dbg.Println(getReady(sc))
		msg := <-sc.Input
		dbg.Println("in client inbox recived")
		//dbg.Println("type:", reflect.TypeOf(logItem{Term: getTerm(sc), Msg: msg}))
		//entry := logItem{Term : getTerm(sc), Msg : msg} 

		sc.currentLogIndex = sc.currentLogIndex + 1
		sc.log = append(sc.log, logItem{Term: getTerm(sc), Msg: msg, MsgId: sc.msgId})

		sc.appendsrcvd = 1
		//setReady(sc, false)
		dbg.Println("sending to outbox")
		sc.inFlight = sc.msgId
		dbg3.Println("currentindex", sc.currentLogIndex)
		sendToCluster(sc, msg)
		//}
	}
}

func sendToCluster(sc *ServerConfig, msg interface{}) {
	for i := 0; i < sc.NoOfPeers; i++ {
		dbg3.Println("sendcluster", sc.nextIndex)
		if sc.currentLogIndex >= sc.nextIndex[i] {
			dbg3.Println("APPEND", &cluster.Envelope{SendTo: sc.basicCluster.Mypeers[i], SendBy: sc.basicCluster.Mypid, MsgId: sc.msgId, Msg: logItem{Term: getTerm(sc), Msg: msg, MsgId: sc.msgId}, Term: getTerm(sc), Type: APPEND, PrevLogIndex: sc.currentLogIndex - 1, PrevLogTerm: sc.log[sc.currentLogIndex-1].Term, LeaderCommit: sc.commitIndex})
			sc.basicCluster.Output <- &cluster.Envelope{SendTo: sc.basicCluster.Mypeers[i], SendBy: sc.basicCluster.Mypid, MsgId: sc.msgId, Msg: logItem{Term: getTerm(sc), Msg: msg, MsgId: sc.msgId}, Term: getTerm(sc), Type: APPEND, PrevLogIndex: sc.currentLogIndex - 1, PrevLogTerm: sc.log[sc.currentLogIndex-1].Term, LeaderCommit: sc.commitIndex}
			sc.msgSend[i] = sc.msgId
			//sc.nextIndex[i] += 1
		}
	}
}

func populateIndexArray(sc *ServerConfig) {
	for i := 0; i < len(sc.nextIndex); i++ {
		sc.nextIndex[i] = len(sc.log)
	}
}

// leaderLoop realizes the logic of LEADER as given in Raft consensus algorithm.
func leaderLoop(sc *ServerConfig) {

	//sc.log = append(sc.log, logItem{getTerm(sc), "" })
	//dbg.Println("logslogs", sc.log, "logs")
	// start heartbeat timer
	// send message to all peers about the aliveness
	populateIndexArray(sc)
	dbg3.Println("leader ban gaya", sc.nextIndex, sc.log)
	sc.basicCluster.Output <- &cluster.Envelope{SendTo: -1, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: HEARTBEAT}
	dbg1.Println(sc.basicCluster.Mypid, "sending heartbeat", -1, getTerm(sc))
	heartTimeChan := time.NewTimer(sc.heartbeatDuration)

	for getState(sc) == LEADER {
		select {
		case <-heartTimeChan.C:
			dbg.Println(sc.basicCluster.Mypid, "sending heartbeat", -1, getTerm(sc))
			// send message to all peers about the aliveness
			sc.basicCluster.Output <- &cluster.Envelope{SendTo: -1, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: HEARTBEAT}
			
			//dbg3.Println("hrtbt", sc.nextIndex)
			for i:=0; i< len(sc.nextIndex); i++ {
				if sc.nextIndex[i] < len(sc.log) && sc.msgSend[i] != sc.log[sc.nextIndex[i]].MsgId {
					dbg3.Println(sc.nextIndex)
					index := sc.nextIndex[i]
					dbg3.Println("send fail hrtbeat", &cluster.Envelope{SendTo: sc.basicCluster.Mypeers[i], SendBy: sc.basicCluster.Mypid, MsgId: sc.log[index].MsgId, Msg: logItem{Term: sc.log[index].Term, MsgId: sc.log[index].MsgId, Msg: sc.log[index].Msg}, Term: getTerm(sc), Type: APPEND, PrevLogIndex: index-1, PrevLogTerm: sc.log[index-1].Term, LeaderCommit: sc.commitIndex})
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: sc.basicCluster.Mypeers[i], SendBy: sc.basicCluster.Mypid, MsgId: sc.log[index].MsgId, Msg: logItem{Term: sc.log[index].Term, MsgId: sc.log[index].MsgId, Msg: sc.log[index].Msg}, Term: getTerm(sc), Type: APPEND, PrevLogIndex: index-1, PrevLogTerm: sc.log[index-1].Term, LeaderCommit: sc.commitIndex}
					//sc.nextIndex[i] += 1
					sc.msgSend[i] = sc.log[index].MsgId
				}
			}
			heartTimeChan = time.NewTimer(sc.heartbeatDuration)
		// wait for input
		case envelope := <-sc.basicCluster.Input:
			dbg.Println("recieve inleader", sc.basicCluster.Mypid, envelope)
			if envelope.Term < getTerm(sc) {
				dbg.Println(sc.basicCluster.Mypid, "send, inleader1", envelope.SendBy, getTerm(sc), "MODIFY")
				sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: MODIFY}
				// if recieved message term is greater than my term, end election, become follower, return
			} else if envelope.Type == HEARTBEAT {
				setTerm(sc, envelope.Term)
				setState(sc, FOLLOWER)
				setVotedFor(sc, NONE)
				setLeader(sc, envelope.SendBy)
				dbg.Println(sc.basicCluster.Mypid, "became follower")
				return
			} else if envelope.Term > getTerm(sc) {
				setTerm(sc, envelope.Term)
				// if message recieved with higher term from leader for vote request, demote itself as follower and grant vote.
				if envelope.Type == REQUESTVOTE {
					setVotedFor(sc, envelope.SendBy)
					dbg.Println(sc.basicCluster.Mypid, "send, inleader2", envelope.SendBy, getTerm(sc), "GRANTVOTE")
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, Term: getTerm(sc), Type: GRANTVOTE, VoteTo: envelope.SendBy}
				}
				setState(sc, FOLLOWER)
				setVotedFor(sc, NONE)
				dbg.Println(sc.basicCluster.Mypid, "became follower again")
				return
			} else if envelope.Type == APPENDSUCCESS {
				dbg3.Println("inapsucc", sc.msgSend, sc.nextIndex)
				if envelope.MsgId == sc.msgSend[findPid(sc, envelope.SendBy)] {
					dbg3.Println("APENDSUCCESS", envelope)
					dbg1.Println("appends", sc.appendsrcvd, "from", envelope.SendBy, "message id", envelope.MsgId)
					sc.nextIndex[findPid(sc, envelope.SendBy)] += 1
					sc.matchIndex[findPid(sc, envelope.SendBy)] += 1
					if envelope.MsgId == sc.inFlight {
						sc.appendsrcvd++
						if sc.appendsrcvd >= sc.majority {
							dbg1.Println("consensus attainedd")
							dbg1.Println("peers", sc.basicCluster.Mypeers, "matchindex", sc.matchIndex)
							//sc.Output <- envelope.Msg
							dbg1.Println(sc.currentLogIndex)
							if sc.log[sc.currentLogIndex].Term == sc.term {
								sc.commitIndex = sc.currentLogIndex
							}
							//setReady(sc, true)
							sc.ready <- true
							sc.inFlight = 0
							sc.msgId += 1
							dbg3.Println("commitindex", sc.commitIndex)
						}
					}
				}
				// if a lower term message is recieved, send a modify message...
			} else if envelope.Type == APPENDFAIL {
				dbg3.Println("APPENDFAIL recieved", envelope)
				if sc.msgSend[findPid(sc, envelope.SendBy)] == envelope.MsgId {
					sc.nextIndex[findPid(sc, envelope.SendBy)] -= 1
					index := sc.nextIndex[findPid(sc, envelope.SendBy)]
					if index == 0 {
						index = 1
						sc.nextIndex[findPid(sc, envelope.SendBy)] = 1
					}
					dbg3.Println("index array", sc.nextIndex)
					dbg3.Println("index", index)
					dbg3.Println("send fail", &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, MsgId: sc.log[index].MsgId, Msg: logItem{Term: sc.log[index].Term, MsgId: sc.log[index].MsgId, Msg: sc.log[index].Msg}, Term: getTerm(sc), Type: APPEND, PrevLogIndex: index-1, PrevLogTerm: sc.log[index-1].Term, LeaderCommit: sc.commitIndex})
					sc.basicCluster.Output <- &cluster.Envelope{SendTo: envelope.SendBy, SendBy: sc.basicCluster.Mypid, MsgId: sc.log[index].MsgId, Msg: logItem{Term: sc.log[index].Term, MsgId: sc.log[index].MsgId, Msg: sc.log[index].Msg}, Term: getTerm(sc), Type: APPEND, PrevLogIndex: index-1, PrevLogTerm: sc.log[index-1].Term, LeaderCommit: sc.commitIndex}
					//sc.nextIndex[findPid(sc, envelope.SendBy)] += 1
					sc.msgSend[findPid(sc, envelope.SendBy)] = sc.log[index].MsgId
				}
			}
			heartTimeChan = time.NewTimer(sc.heartbeatDuration)
		// If error.	
		case <-sc.ErrorChannel:
			closeServer(sc)
		}
	}

}

// findPid returns the index of server's peers array which correspond to the given pid. If not found, returns -1
func findPid(sc *ServerConfig, pid int) int {
	for i := 0; i < len(sc.basicCluster.Mypeers); i++ {
		if pid == sc.basicCluster.Mypeers[i] {
			return i
		}
	}
	return -1
}
