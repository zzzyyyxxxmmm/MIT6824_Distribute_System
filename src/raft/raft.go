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

/*
reply的term是用来做什么的???
 */
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

func (raft *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {

	raft.mu.Lock()
	defer raft.mu.Unlock()
	//reply false if term < currentTerm
	reply.VoteGranted = false

	/*
	If a candidate or leader discovers that its term is out of date, it immediately reverts to follower state
	 */
	if (raft.State == Candidate || raft.State == Leader) && raft.CurrentTerm < args.Term {
		raft.BeFollower(args.Term)
		return
	}

	if args.Term < raft.CurrentTerm {
		return
	}

	//if votedFor is null or candidateId, and candidate's log is at least as up-to-date as receiver's log, grant vote -> candidate.log >= rf.log
	// enter to vote
	// A server remains in f
	if (raft.VoteFor == -1 || raft.VoteFor == args.CandidateId) && args.LastLogIndex >= raft.GetLastLogIndex() {
		log.Info("RequestVote", "message", raft.me, " vote to ", args.CandidateId)
		reply.VoteGranted = true
		reply.Term = raft.CurrentTerm
		send(raft.voteCh)
	}
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	//1. reply false if term < currentTerm
	reply.Success = false
	reply.Term = rf.CurrentTerm
	rf.CurrentTerm=args.Term

	//fmt.Println(args.Term,rf.CurrentTerm)
	if args.Term < rf.CurrentTerm {
		return
	}
	reply.Success = true
	send(rf.appendLogCh)
}

func (raft *Raft) sendRequestVote(server int, args RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := raft.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (raft *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

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
	rf.voteCh=make(chan bool ,1 )
	rf.appendLogCh=make(chan bool, 1 )

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
					log.Info("start election", "message: ", rf.me, "start election")
					rf.BeCandidate()
				case <-rf.appendLogCh:
					log.Info("election time refresh", "appendLogCh",rf.me)
				case <-rf.voteCh:
					log.Info("election time refresh", "voteCh",rf.me)
				}

			case Leader:
				time.Sleep(time.Duration(100) * time.Millisecond)
				//rf.StartAppendLog()
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
	for i := 0; i < len(raft.peers); i++ {
		if i == raft.me {
			continue
		}

		go func(id int) {
			reply := RequestVoteReply{}
			result := raft.sendRequestVote(id, arg, &reply)

			if result {
				raft.mu.Lock()
				defer raft.mu.Unlock()
				if reply.VoteGranted {
					atomic.AddInt32(&count, 1)
				}

				if raft.State==Leader{
					return
				}

				if atomic.LoadInt32(&count) > int32(len(raft.peers)/2) {
					log.Info("become leader", "message", raft.me)
					raft.BeLeader()
					go raft.StartAppendLog()
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
	raft.mu.Lock()
	raft.State = Follower
	raft.mu.Unlock()
}
func (raft *Raft) BeLeader() {
	raft.State = Leader
}
func (raft *Raft) StartAppendLog() {
	log.Info("start sending append log","",raft.me)
	arg := AppendEntriesArgs{
		Term:     raft.CurrentTerm,
		LeaderId: raft.me,
	}

	for i:=0;i<len(raft.peers);i++{
		if i == raft.me {
			continue
		}

		go func(id int) {

			for {
				reply := AppendEntriesReply{}
				raft.sendAppendEntries(id, &arg, &reply)
				time.Sleep(time.Duration(100)*time.Millisecond)
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
