KVStore
=========

KVStore is a go language implementation of **Raft consensus protocol**. This protocol is mainly used for synchronization between different servers using message passing technique.

The algorithm implemented for synchronization is given in this paper:

[In Search of an Understandable Consensus Algorithm](https://ramcloud.stanford.edu/wiki/download/attachments/11370504/raft.pdf) given by Diego Ongaro and John Ousterhout

The primary aim of this project is to make this library robust. Test cases are included for the same.

Currently, **Leader election process** and **Log Replication** is implemented to its full extent.



How KVStore works??
------------------------
KVStore comprises of two main parts:
1. Raft implementation
2. Database Store

It works as follows:
At start the system reaches a consensus state using Raft leader election. At this point, client sends request to the leader, mainly for setting the values in database. Leader using the log replication mechanism, replicates it on all its peers. Once it gets replicated, it applies the same to its state machine. Not only the leader, but all the peers periodically applies the data to state machine. Now when next time client request such data from any of the server, its request can be fulfilled.

Raft protocol
----------

As a part of Raft consensus protocol, two things needs to be implemented:
1. Leader Election
2. Log Replication

LEADER ELECTION

This process can be summarized as follows.

KVStore works as client-server system and all the activities are managed by the server itself. For that a leader is elected amongs all the peers, and once elected, it will be responsible for all synchronization related activites, unless something unusual happens.

Inititally all servers start as FOLLOWER. Raft ensures that ther is only one leader at any particular instant. For leader election process, server 
promote themselves as CANDIDATE and request for votes from its peers. Only after getting majority of votes (quorum size) can the candidate become LEADER.

The following figure specifies the whole server states and how they can move from one state to other.

![Alt text](https://f.cloud.github.com/assets/6353786/2181747/b53f24d4-9763-11e3-8cd3-3c56dc28a6f9.png)

LOG REPLICATION

After getting elected as Leader, the server starts accepting client requests. Once it recieves a client request, it forwards the request to all its peers and creates the log entry for the same. Peers on getting such request, stores the message along with leader term in log, and respond to the leader. On getting response from majority of peers (quorum size), leader marks this entry as committed, which will be notified to other peers in the next round.
Thus, it maintains consistency among all the peers.

TODO
-------------

Many things need to be added to it to make a complete Raft library. However, in the current implementation also, some things can be added to make it more robust.

1. Heavy stress testing, where servers are going down and waking up very quickly. 

Usage
--------------

To retrieve the repository from github, use: 
```sh
go get github.com/vibhor1403/KVStore
```
To test, I have assumed a folder named KVStore in GOPATH directory. This folder should contain the config file, and log files, initially empty.
As, I have tested for 5 servers, and all the systems are running on local machine, 5 log files should be created in the same directory. Simply copy the KVStore folder provided in the repository to GOPATH.

To test the cluster library, use:
```sh
go test -v github.com/vibhor1403/KVStore
```
This will test the library on all aspects, considering 5 servers passing messages between each other.

To run a single instance of server, use:
```sh
KVStore <pid> <configFilePath> <logFilePath> <databaseStorePath>
```

This pid should be present in the config.json file. This will start the server and broadcast a message to all its peers.


Tweaking
-----------


Few constants are defined in the Raft.go file which sets the timeout duration and heartbeat interval. They can be changed according to the network situtaion. However majority of code testing is done with the following default values:

* `timeoutDuration = 200 miillisecond` - It determines the base duration after which follower starts a new election if no message is recieved from other servers. The actual duration is kept a bit random so that all servers don't start election at same time.

* `heartbeatinterval    = 50 millisecond` - Duration in which leader sends keep alive messages to its peers.

* `recieveTO = 2 second` - Timeout for listen socket. After this timeout the listen socket will get closed, if nothing is recieved on it during this time.

--------------------

For testing purpose also, the following constants are defined in Raft_test.go and can be altered accordingly:

* `toleranceDuration = 5 seconds` - Maximum tolerance in which if no leader is elected, test fails.

* `pingDuration		= 100 milliseconds` - After every pingDuration current leader is found out, and if more than one leader remains, test fails.

* `testDuration		= 10 seconds` - Total time for which test cases need to run.

JSON format
----------------
This file contains the pid and url of all servers in the cluster. It is required to give the value of total correct.
```sh
{"object": 
        {
           	"total": 2,
       		"Servers":
       		[
               		{
                       		"mypid": 1,
                       		"url": "127.0.0.1:100001"
                       	},
			{
                       		"mypid": 2,
                       		"url": "127.0.0.1:100002"
                       	}
       		]
    	}
}
```
