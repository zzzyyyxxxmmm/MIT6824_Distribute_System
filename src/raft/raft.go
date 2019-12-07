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
	log "github.com/inconshreveable/log15"
	"sync/atomic"
	"sort"
	"bytes"
	"encoding/gob"
	"math/rand"
	"time"
)

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
	applyCh     chan ApplyMsg
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

type InstallSnapshotArgs struct {
	Term              int    "leader’s term"
	LeaderId          int    "so follower can redirect clients"
	LastIncludedIndex int    "the snapshot replaces all entries up through and including this index"
	LastIncludedTerm  int    "term of lastIncludedIndex"
	Offset            int    "byte offset where chunk is positioned in the snapshot file"
	Data              []byte "raw bytes of the snapshot chunk, starting at offset"

	done bool "true if this is the last chunk"
}

type InstallSnapshotReply struct {
	Term int
}

//
// save Raft's persistent State to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	w := new(bytes.Buffer)
	e := gob.NewEncoder(w)
	e.Encode(rf.CurrentTerm)
	e.Encode(rf.VoteFor)
	e.Encode(rf.Log)
	data := w.Bytes()
	rf.persister.SaveRaftState(data)
}

//
// restore previously persisted State.
//
func (rf *Raft) readPersist(data []byte) {
	r := bytes.NewBuffer(data)
	d := gob.NewDecoder(r)
	d.Decode(&rf.CurrentTerm)
	d.Decode(&rf.VoteFor)
	d.Decode(&rf.Log)
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

func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {

	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.CurrentTerm

	//1. reply immediately if term < currentTerm
	if args.Term < rf.CurrentTerm {
		return
	}

	if rf.CurrentTerm < args.Term {
		rf.BeFollower(args.Term)
	}

}

func (raft *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {

	raft.mu.Lock()
	defer raft.mu.Unlock()
	//reply false if term < currentTerm

	if raft.CurrentTerm < args.Term {
		raft.BeFollower(args.Term)
	}

	reply.Term = raft.CurrentTerm
	reply.VoteGranted = false

	if args.Term < raft.CurrentTerm {
		return
	}

	//If votedFor is null or candidateId, and candidate's log is at least as up-to-date as receiver's log, grant vote
	//Raft determine which of two logs is more up-to-data by comparing the index and term of the last entries in the logs. If the logs have last
	// entries with different terms, then the log with the later term is more up-to-date. If the logs end with the same term, then whichever log is longer
	//is more up-to-date.
	//这里遇到的问题就是, 非leader server断开, term会一直增长, 尽管这个server重连后term大, 会触发election, 但是它的log短, 因此不会获得其他成员的投票
	if raft.VoteFor != -1 && raft.VoteFor != args.CandidateId {

	} else if args.LastLogTerm < raft.Log[len(raft.Log)-1].Term || (args.LastLogTerm == raft.Log[len(raft.Log)-1].Term && args.LastLogIndex < len(raft.Log)-1) {

	} else {
		log.Info("RequestVote", "id", raft.me, " vote to ", args.CandidateId)
		reply.VoteGranted = true
		raft.VoteFor = args.CandidateId
		send(raft.voteCh)
		raft.persist()
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

	//term小的leader发送给term大的follower, leader应该变成follower, 正常情况下不用处理, 因为term小的leader会被term大的leader的heartbeat找到, 然后使其
	//变成follower, 如上, 但是如果term小的leader加入的瞬间, term大的leader断开连接, 这时候就无法处理了
	if args.Term < rf.CurrentTerm {
		return
	}

	//2. Reply false if log doesn't contain an entry at prevLogIndex whose term matches prevLogTerm
	if args.PrevLogIndex >= len(rf.Log) {
		return
	}
	//3. If an existing entry conflicts with a new one (same index but different terms), delete the existing entry and all that follow it.
	preLogTerm := rf.Log[args.PrevLogIndex].Term


	if preLogTerm != args.PrevLogTerm {
		return
	}

	//4. Append any new entries not already in the log
	// 这里千万不能直接append, 比如, 123同步到了所有节点上,这时候产生了一个新leader, 这个leader的nextIndex是1, 会发送123, 这时候就会变成123123
	rf.Log = append(rf.Log[:args.PrevLogIndex+1], args.Entries...)

	//5. If leaderCommit > commitIndex, set commitIndex = min(leaderCommit, index of last new entry)
	if args.LeaderCommit > rf.CommitIndex {
		rf.CommitIndex = min(args.LeaderCommit, len(rf.Log)-1)
		rf.updateLastApplied()
	}
	rf.persist()

	log.Info("follower start to replicate log", "follower", rf.me, "Log", rf.Log, "commitIndex", rf.CommitIndex, "lastApplied", rf.LastApplied, "PrevLogIndex", args.PrevLogIndex)

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

func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
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
		log.Info("<<<<<<<<<<<<<---------------receive command", "src", raft.me, "command", command)
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
	//log.Info("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!Kill","id",raft.me)
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
		LastLogIndex: len(raft.Log) - 1,
		LastLogTerm:  raft.Log[len(raft.Log)-1].Term,
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

				if reply.Term > raft.CurrentTerm {
					raft.BeFollower(reply.Term)
					return
				}

				if raft.State != Candidate || raft.CurrentTerm != arg.Term {
					return
				}

				if reply.VoteGranted {
					atomic.AddInt32(&count, 1)
				}

				if atomic.LoadInt32(&count) > int32(len(raft.peers)/2) {

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
	log.Info("become follower", "id", raft.me, "term", raft.CurrentTerm)
	raft.CurrentTerm = term
	raft.State = Follower
	raft.VoteFor = -1

}

func (raft *Raft) BeLeader() {
	log.Info("!!!!!!become leader!!!!!!", "who", raft.me)
	raft.State = Leader

	raft.NextIndex = make([]int, len(raft.peers))
	raft.MatchIndex = make([]int, len(raft.peers))
	for i := 0; i < len(raft.NextIndex); i++ { //initialized to leader last log index + 1
		raft.NextIndex[i] = len(raft.Log)
	}
}

func (raft *Raft) StartAppendLog() {
	log.Info("leader start send heartbeat leader log information", "who is leader", raft.me, "term", raft.CurrentTerm, "log", raft.Log, "commitIndex", raft.CommitIndex, "lastApplied", raft.LastApplied)

	for i, _ := range raft.peers {
		if i == raft.me {
			continue
		}
		go func(id int) {
			for {
				raft.mu.Lock()
				//旧的leader重新加入的时候如果变成follower, 这里的循环还是存在的, 因为某个节点失联会一直continue重连, 因此必须加上state判断
				if raft.State != Leader {
					raft.mu.Unlock()
					return
				}
				arg := AppendEntriesArgs{
					Term:         raft.CurrentTerm,
					LeaderId:     raft.me,
					PrevLogIndex: raft.NextIndex[id] - 1,
					PrevLogTerm:  raft.Log[raft.NextIndex[id]-1].Term,
					//Entries:      append(make([]Log, 0), raft.getLog(raft.getPrevLogIdx(id))),
					LeaderCommit: raft.CommitIndex,
				}
				// If last log index >= nextIndex for a follower: send AppendEntries RPC with log entries RPC with log entries starting at nextIndex
				// 这里需要解释一下初始情况, 初始是leader有一个nil Log, index是0, nextIndex是1, 在leader新加了一个log后, leader最后一个log index变成1 >= nextIndex, 因此
				//需要将这个log复制, 但如果log是两个的话, 显然还是要根据nextIndex来复制
				//但是在插入之前需要保证之前的Log是同步的
				//fmt.Println("************",len(raft.Log),"   ",raft.NextIndex[id])
				if len(raft.Log)-1 >= raft.NextIndex[id] {
					arg.Entries = append(make([]Log, 0), raft.Log[raft.NextIndex[id]:]...)
				} else {
					arg.Entries = make([]Log, 0)
				}

				if len(arg.Entries) > 0 {
					log.Info("replicating log", "to whom",id, "arg", arg,"nextIndex",raft.NextIndex[id])
				}

				raft.mu.Unlock()
				reply := AppendEntriesReply{}
				ret := raft.sendAppendEntries(id, &arg, &reply)
				raft.mu.Lock()

				if !ret {
					log.Info("append entries fail to send", "leaderId",raft.me,"to whom", id)
					raft.mu.Unlock()
					return
				}

				//term小的leader发送给term大的follower
				if reply.Term > raft.CurrentTerm {
					raft.BeFollower(reply.Term)
					raft.mu.Unlock()
					return
				}

				//if successful: update nextIndex and matchIndex for follower
				if reply.Success {
					raft.MatchIndex[id] = len(raft.Log) - 1
					raft.NextIndex[id] = raft.MatchIndex[id] + 1

					//If there exists an N such that N > commitIndex, a majority of matchIndex[i]>=N, and log[N].term==currentTerm
					//set commitIndex=N
					//如果至少有一半, 那么中位数一定是那个大于一半的
					raft.MatchIndex[raft.me] = len(raft.Log) - 1
					copyMatchIndex := make([]int, len(raft.MatchIndex))
					copy(copyMatchIndex, raft.MatchIndex)
					sort.Sort(sort.Reverse(sort.IntSlice(copyMatchIndex)))
					N := copyMatchIndex[len(copyMatchIndex)/2]
					if N > raft.CommitIndex && raft.Log[N].Term == raft.CurrentTerm {

						raft.CommitIndex = N
						log.Info("start to commit info", "id", raft.me, "commitIndex", raft.CommitIndex)
						raft.updateLastApplied()
					}
				} else {
					//If AppendEntries fails because of log inconsistency: decrement nextIndex and retry
					raft.NextIndex[id]--
					raft.mu.Unlock()
					continue
				}
				raft.mu.Unlock()

				return
			}

		}(i)
	}
}


func Make(peers []*labrpc.ClientEnd, me int, persister *Persister, applyCh chan ApplyMsg) *Raft {
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
	rf.applyCh = applyCh

	rf.CommitIndex = 0
	rf.LastApplied = 0
	rf.Log = make([]Log, 1) //first index is 1
	rf.Log[0] = Log{
		Term:    -1,
		Command: nil,
	}

	rf.readPersist(persister.ReadRaftState())

	go func() {
		for {
			electionTimeout := time.Duration(rand.Intn(200)+300) * time.Millisecond
			rf.mu.Lock()
			state := rf.State
			rf.mu.Unlock()

			switch state {
			case Follower, Candidate:
				select {
				case <-rf.voteCh:
					log.Info("election time refresh", "voteCh", rf.me)
				case <-rf.appendLogCh:
					log.Info("election time refresh", "appendLogCh", rf.me)
				case <-time.After(electionTimeout):
					log.Info("start election", "id", rf.me, "term", rf.CurrentTerm+1)
					rf.mu.Lock()
					rf.BeCandidate()
					rf.mu.Unlock()

				}

			case Leader:
				rf.StartAppendLog()
				time.Sleep(time.Duration(100) * time.Millisecond)
			}

		}
	}()

	return rf
}

func (raft *Raft) updateLastApplied() {
	for raft.LastApplied < raft.CommitIndex {
		raft.LastApplied++
		curLog := raft.Log[raft.LastApplied]
		applyMsg := ApplyMsg{raft.LastApplied, curLog.Command, false, nil}
		raft.applyCh <- applyMsg
	}
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func send(ch chan bool) {
	select {
	case <-ch: //if already set, consume it then resent to avoid block
	default:}
	ch <- true
}