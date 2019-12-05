package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"sync"
	"labrpc"
	"math/rand"
	//"github.com/labstack/gommon/log"
	"time"
	log "github.com/inconshreveable/log15"
	"sync/atomic"
	"sort"
)

// import "bytes"
// import "encoding/gob"

const (
	Follower  = iota + 1
	Candidate
	Leader
)

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex
	peers     []*labrpc.ClientEnd
	persister *Persister
	me        int // index into peers[]

	//persistent state on all servers
	State       int
	CurrentTerm int
	VoteFor     int
	Log         []Log

	//volatile state on all servers
	CommitIndex int
	LastApplied int

	//volatile state on leaders
	NextIndex  []int
	MatchIndex []int

	appendLogCh chan bool
	voteCh      chan bool
	applyCh chan ApplyMsg
}

type Log struct {
	Term    int
	Command interface{}
}

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make().
//
type ApplyMsg struct {
	Index       int
	Command     interface{}
	UseSnapshot bool   // ignore for lab2; only used in lab3
	Snapshot    []byte // ignore for lab2; only used in lab3
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

// return currentTerm and whether this server
// believes it is the leader.
func (raft *Raft) GetState() (int, bool) {

	var term int
	var isleader bool
	raft.mu.Lock()
	defer raft.mu.Unlock()
	term = raft.CurrentTerm
	isleader = (raft.State == Leader)
	return term, isleader
}

//
// save Raft's persistent State to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (raft *Raft) persist() {
	// Your code here.
	// Example:
	// w := new(bytes.Buffer)
	// e := gob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted State.
//
func (raft *Raft) readPersist(data []byte) {
	// Your code here.
	// Example:
	// r := bytes.NewBuffer(data)
	// d := gob.NewDecoder(r)
	// d.Decode(&rf.xxx)
	// d.Decode(&rf.yyy)
}

type RequestVoteArgs struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []Log
	LeaderCommit int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

func (raft *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {

	raft.mu.Lock()
	defer raft.mu.Unlock()
	//reply false if term < currentTerm

	if raft.CurrentTerm < args.Term {
		log.Info("requestVote stage", "id", raft.me, "org", raft.State, "src", "follower")
		raft.BeFollower(args.Term)
	}

	reply.Term = raft.CurrentTerm
	reply.VoteGranted = false

	if args.Term < raft.CurrentTerm {
		return
	}

	if raft.VoteFor == -1 || raft.VoteFor == args.CandidateId {
		log.Info("RequestVote", "id", raft.me, " vote to ", args.CandidateId)
		reply.VoteGranted = true
		reply.Term = raft.CurrentTerm
		send(raft.voteCh)
	}

}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {

	rf.mu.Lock()
	defer rf.mu.Unlock()

	//1. reply false if term < currentTerm
	if args.Term > rf.CurrentTerm {
		rf.BeFollower(args.Term)
	}

	send(rf.appendLogCh) //只要发送了就一定会被刷新

	reply.Success = false
	reply.Term = rf.CurrentTerm

	//2. Reply false if log doesn't contain an entry at prevLogIndex whose term matches prevLogTerm
	if args.PrevLogIndex>=len(rf.Log){
		return
	}

	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it.
	preLogTerm := rf.Log[args.PrevLogIndex].Term
	if preLogTerm != args.PrevLogTerm {
		return
	}

	//4. Append any new entries not already in the log
	if len(args.Entries)>0{
		rf.Log = append(rf.Log, args.Entries...)

		}


	//5. If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
	if args.LeaderCommit > rf.CommitIndex {
		rf.CommitIndex = min(args.LeaderCommit, len(rf.Log))
		rf.updateLastApplied()
	}

	log.Info("follower start to replicate log", "follower", rf.me,"Log",rf.Log,"commitIndex",rf.CommitIndex,"lastApplied",rf.LastApplied)

	reply.Success = true

}

func (raft *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := raft.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (raft *Raft) Start(command interface{}) (int, int, bool) {
	raft.mu.Lock()
	defer raft.mu.Unlock()
	index := -1
	term := raft.CurrentTerm
	isLeader := raft.State == Leader
	//If command received from client: append entry to local log, respond after entry applied to state machine (§5.3)

	if isLeader {
		log.Info("receive command", "src",raft.me,"command",command)
		index = raft.GetLastLogIndex() + 1
		newLog := Log{
			raft.CurrentTerm,
			command,
		}
		raft.Log = append(raft.Log, newLog)
		raft.persist()
		//raft.StartAppendLog()
	}
	return index, term, isLeader
}

func (raft *Raft) Kill() {
	// Your code here, if desired.
}

func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	log.Info("start a server", "index", me)
	// Your initialization code here.

	// initialize from State persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	rf.State = Follower
	rf.CurrentTerm = 1
	rf.VoteFor = -1
	rf.voteCh = make(chan bool, 1)
	rf.appendLogCh = make(chan bool, 1)
	rf.applyCh=applyCh

	rf.CommitIndex=0
	rf.LastApplied=0
	rf.Log=make([]Log,1)	//first index is 1
	rf.Log[0]=Log{
		Term:-1,
		Command:nil,
	}

	go func() {
		for {
			electionTimeout := time.Duration(rand.Intn(200)+300) * time.Millisecond
			rf.mu.Lock()
			state := rf.State
			rf.mu.Unlock()

			switch state {
			case Follower, Candidate:
				select {
				case <-time.After(electionTimeout):
					log.Info("start election", "id", rf.me, "term", rf.CurrentTerm+1)
					rf.BeCandidate()
				case <-rf.appendLogCh:
					log.Info("election time refresh", "appendLogCh", rf.me)
				case <-rf.voteCh:
					log.Info("election time refresh", "voteCh", rf.me)
				}

			case Leader:
				rf.StartAppendLog()
				time.Sleep(time.Duration(100) * time.Millisecond)
			}

		}
	}()

	return rf
}
func (raft *Raft) BeCandidate() {
	raft.CurrentTerm++ //increment currentTerm
	raft.State = Candidate
	raft.VoteFor = raft.me
	go raft.startElection()
}
func (raft *Raft) startElection() {
	//sendVoteRequest to it's peers
	arg := RequestVoteArgs{
		Term:         raft.CurrentTerm,
		CandidateId:  raft.me,
		LastLogIndex: raft.GetLastLogIndex(),
		LastLogTerm:  -1,
	}

	var count int32 = 1;
	for i, _ := range raft.peers {

		if i == raft.me {
			continue
		}
		//log.Info("candidate send request vote to others","src",raft.me,"dst",i,"term",raft.CurrentTerm)
		go func(id int) {
			reply := RequestVoteReply{}
			result := raft.sendRequestVote(id, arg, &reply)

			if result {
				raft.mu.Lock()
				defer raft.mu.Unlock()
				if reply.VoteGranted {
					atomic.AddInt32(&count, 1)
				}

				if raft.State == Leader {
					return
				}

				if atomic.LoadInt32(&count) > int32(len(raft.peers)/2) {
					log.Info("become leader", "message", raft.me)
					raft.BeLeader()
					send(raft.voteCh)
				}
			}

		}(i)
	}
}
func (raft *Raft) GetLastLogIndex() int {
	return len(raft.Log) - 1
}
func (raft *Raft) BeFollower(term int) {
	raft.CurrentTerm = term
	raft.State = Follower
	raft.VoteFor = -1
}
func (raft *Raft) BeLeader() {
	raft.State = Leader

	raft.NextIndex = make([]int, len(raft.peers))
	raft.MatchIndex = make([]int, len(raft.peers))
	for i := 0; i < len(raft.NextIndex); i++ { //initialized to leader last log index + 1
		raft.NextIndex[i] = len(raft.Log)
	}
}
func (raft *Raft) StartAppendLog() {
	log.Info("start sending append log", "leader", raft.me, "current term", raft.CurrentTerm)

	log.Info("leader log information","log",raft.Log,"commitIndex",raft.CommitIndex,"lastApplied",raft.LastApplied)

	for i, _ := range raft.peers {
		if i == raft.me {
			continue
		}
		go func(id int) {
			for {
				arg := AppendEntriesArgs{
					Term:         raft.CurrentTerm,
					LeaderId:     raft.me,
					PrevLogIndex: raft.NextIndex[id]-1,
					PrevLogTerm:  raft.Log[raft.NextIndex[id]-1].Term,
					//Entries:      append(make([]Log, 0), raft.getLog(raft.getPrevLogIdx(id))),
					LeaderCommit: raft.CommitIndex,
				}
				// If last log index >= nextIndex for a follower: send AppendEntries RPC with log entries RPC with log entries starting at nextIndex
				// 这里需要解释一下初始情况, 初始是leader有一个nil Log, index是0, nextIndex是1, 在leader新加了一个log后, leader最后一个log index变成1 >= nextIndex, 因此
				//需要将这个log复制, 但如果log是两个的话, 显然还是要根据nextIndex来复制
				//但是在插入之前需要保证之前的Log是同步的
				if len(raft.Log)-1 >= raft.NextIndex[id]{
					arg.Entries=append(make([]Log,0),raft.Log[raft.NextIndex[id]])
				} else {
					arg.Entries=make([]Log,0)
				}

				if len(arg.Entries)>0{
					log.Info("replicating log","arg",arg)
				}

				reply := AppendEntriesReply{}
				ret := raft.sendAppendEntries(id, &arg, &reply)

				if !ret {
					time.Sleep(time.Duration(300) * time.Millisecond)
					continue
				}

				// This is only heartbeat
				if len(arg.Entries)==0{
					return
				}



				//if successful: update nextIndex and matchIndex for follower
				if reply.Success{
					raft.MatchIndex[id]=raft.NextIndex[id]
					raft.NextIndex[id]++

					//If there exists an N such that N > commitIndex, a majority of matchIndex[i]>=N, and log[N].term==currentTerm
					//set commitIndex=N
					//如果至少有一半, 那么中位数一定是那个大于一半的
					raft.MatchIndex[raft.me]=len(raft.Log)-1
					copyMatchIndex := make([]int,len(raft.MatchIndex))
					copy(copyMatchIndex,raft.MatchIndex)
					sort.Sort(sort.Reverse(sort.IntSlice(copyMatchIndex)))
					N := copyMatchIndex[len(copyMatchIndex)/2]
					if N > raft.CommitIndex && raft.Log[N].Term == raft.CurrentTerm {
						raft.CommitIndex = N
						raft.updateLastApplied()
					}
				} else {
					//If AppendEntries fails because of log inconsistency: decrement nextIndex and retry
					raft.NextIndex[id]--
					continue
				}

				return
			}

		}(i)
	}
}

func send(ch chan bool) {
	select {
	case <-ch: //if already set, consume it then resent to avoid block
	default:}
	ch <- true
}

func (raft *Raft) updateLastApplied() {
	for raft.LastApplied < raft.CommitIndex {
		raft.LastApplied++
		curLog := raft.Log[raft.LastApplied]
		applyMsg := ApplyMsg{ raft.LastApplied, curLog.Command,false,nil}
		log.Info("command submitted", "src",raft.me)
		raft.applyCh <- applyMsg
	}
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}
