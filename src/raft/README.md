# BackGround

Thesis Link : [link](http://nil.csail.mit.edu/6.824/2016/papers/raft-extended.pdf)

Raft implements consensus by first electing a distinguished leader, then giving the leader complete responsibility for managing the replicated log. The leader accepts
log entries from clients, replicates them on other servers,
and tells servers when it is safe to apply log entries to
their state machines. Having a leader simplifies the management of the replicated log. For example, the leader can
decide where to place new entries in the log without consulting other servers, and data flows in a simple fashion
from the leader to other servers. A leader can fail or become disconnected from the other servers, in which case
a new leader is elected.

Given the leader approach, Raft decomposes the consensus problem into three relatively independent subproblems, which are discussed in the subsections that follow:
* **Leader election**: a new leader must be chosen when
an existing leader fails (Section 5.2).
* **Log replication**: the leader must accept log entries
from clients and replicate them across the cluster,
forcing the other logs to agree with its own (Section 5.3).
* **Safety**: the key safety property for Raft is the State
Machine Safety Property in Figure 3: if any server
has applied a particular log entry to its state machine,
then no other server may apply a different command
for the same log index. Section 5.4 describes how
Raft ensures this property; the solution involves an
additional restriction on the election mechanism described in Section 5.2.

# 思路

首先这个[illustrated Raft guide](http://thesecretlivesofdata.com/raft/)提供了很好理解的执行过程，再配上论文里的的[figure2](https://github.com/zzzyyyxxxmmm/MIT6824_Distribute_System/tree/master/img/raft01.png)，我们应该可以开始对代码进行分析了.

# Task I Implement leader election and heartbeats

Implement leader election and heartbeats (empty AppendEntries calls). This should be sufficient for a single leader to be elected, and to stay the leader, in the absence of failures. Once you have this working, you should be able to pass the first two "go test" tests.

Add any state you need to keep to the Raft struct in raft.go. Figure 2 in the paper may provide a good guideline. You'll also need to define a struct to hold information about each log entry. Remember that the field names any structures you will be sending over RPC must start with capital letters, as must the field names in any structure passed inside an RPC.

You should start by implementing Raft leader election. Fill in the RequestVoteArgs and RequestVoteReply structs, and modify Make() to create a background goroutine that starts an election (by sending out RequestVote RPCs) when it hasn't heard from another peer for a while. For election to work, you will also need to implement the RequestVote() RPC handler so that servers will vote for one another.

To implement heartbeats, you will need to define an AppendEntries RPC structs (though you may not need all the arguments yet), and have the leader send them out periodically. You will also have to write an AppendEntries RPC handler method that resets the election timeout so that other servers don't step forward as leaders when one has already been elected.

Make sure the timers in different Raft peers are not synchronized. In particular, make sure the election timeouts don't always fire at the same time, or else all peers will vote for themselves and no one will become leader.

其实Hints已经交代的比较清楚了，回顾一下leader election的流程
1. timeout最先到的raft会发送RequestVote
2. 接收到RequestVote的raft发送RequestVoteReply
3. 发送方接收到返回的RequestVoteReply后，统计数目
4. 如果超过大多数，则获胜成为leader
5. 发送heartbeats给其他raft

在test.test.go中，会执行cfg.checkoneleader()
```go
func (cfg *config) checkOneLeader() int {
	for iters := 0; iters < 10; iters++ {
		time.Sleep(500 * time.Millisecond)
		leaders := make(map[int][]int)
		for i := 0; i < cfg.n; i++ {
			if cfg.connected[i] {
				if t, leader := cfg.rafts[i].GetState(); leader {
					leaders[t] = append(leaders[t], i)
				}
			}
		}

		lastTermWithLeader := -1
		for t, leaders := range leaders {
			if len(leaders) > 1 {
				cfg.t.Fatalf("term %d has %d (>1) leaders", t, len(leaders))
			}
			if t > lastTermWithLeader {
				lastTermWithLeader = t
			}
		}

		if len(leaders) != 0 {
			return leaders[lastTermWithLeader][0]
		}
	}
	cfg.t.Fatalf("expected one leader, got none")
	return -1
}
```
里面循环检查了每个raft的状态是否是leader状态，如果是则加入到leader里，最终的leader只能有一个，因此我们需要修改cfg.rafts.GetState()使其在10个iter中有且仅出现一个返回是true的情况

## 定义状态
直接用枚举定义出三种状态：
```go
type State int
const (
	Follower State = iota
	Candidate
	Leader
)
```

在getState()中添加：
```go
isleader=(rf.state==Leader)
```

ok，现在我们需要进入发起投票环节，那么先定义发送了什么
```
Invoked by candidates to gather votes (§5.2).
Arguments:
term candidate’s term
candidateId candidate requesting vote
lastLogIndex index of candidate’s last log entry (§5.4)
lastLogTerm term of candidate’s last log entry (§5.4)

Results:
term currentTerm, for candidate to update itself
voteGranted true means candidate received vote

Receiver implementation:
1. Reply false if term < currentTerm (§5.1)
2. If votedFor is null or candidateId, and candidate’s log is at
least as up-to-date as receiver’s log, grant vote (§5.2, §5.4)
```

其实一口气出现这么多参数很不好理解，等我们需要用到的时候再加上去

```go
type RequestVoteArgs struct {
	// Your data here.
	CandidateId int
}

type RequestVoteReply struct {
	// Your data here.
	VoteGranted bool
}
```

## 发送投票请求
接下来就可以开始进行选举了：
```go
func (rf *Raft) startElection(){
	args:=RequestVoteArgs{
		rf.me,
	}
	var votes int32 = 1;
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}
		go func(idx int) {
			reply := &RequestVoteReply{}
			ret := rf.sendRequestVote(idx,&args,reply)

			if ret {
				if reply.VoteGranted {
					atomic.AddInt32(&votes,1)
				} //If votes received from majority of servers: become leader
				if atomic.LoadInt32(&votes) > int32(len(rf.peers) / 2) {
					rf.winElection()
				}
			}
		}(i)
	}
} 

func (ff *Raft) winElection(){
	rf.state=Leader
}
```

## 接收请求
```go
func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.
	reply.VoteGranted = true
}
```

我们一次给所有peers发信息，信息会通过RequestVote()，方法被接收到并返回结果，如果选举获胜，那么该candidate就可以成为leader了

raft是在Make方法里启动的，因此我们在这里启动startElection()
```go
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

    // Your initialization code here.
    fr.state=Follower

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	rf.startElection()
	return rf
}
```

到目前为止大概完成了一个粗略的model，这里如果直接跑的话，会出现term 0 has 3 (>1) leaders的错误，因为每个raft都进行了选举，都接收到了投票，都成为了leader，因此，我们需要避免这样的情况

## 如何只选出一位Leader
Once a candidate wins an election, it
becomes leader. It then sends heartbeat messages to all of
the other servers to establish its authority and prevent new
elections.

从这里我们可以知道，一旦成为了Leader，则需要发出heartbeat messages(AppendEntries)来通知其他candidate使其放弃竞选,heartbeat是周期性的发出去的，因此我们需要在另一个线程的无限
循环里周期性发送message，那么到底怎么知道其他raft有没有成为Leader呢，这里我们需要引入term的概念：

Raft算法将时间分为一个个的任期（term），每一个term的开始都是Leader选举。在成功选举Leader之后，Leader会在整个term内管理整个集群。如果Leader选举失败，该term就会因为没有Leader而结束。

因此我们发送信息的时候，可以拿自己的term和发送方的term比较，如果对方的term比自己的大，说明对方已经竞选成功了，进入下一个任期了

我们在所有发送log的信息里加入term

```go

func (rf *Raft) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here.

	reply.Term = rf.currentTerm
	reply.VoteGranted = false

	if args.Term < rf.currentTerm {

	} else {
		reply.VoteGranted = true
		rf.state = Follower
	}

}

//heartbeat
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	if args.Term > rf.currentTerm {
		rf.beFollower(args.Term)
	}
	reply.Term = rf.currentTerm
}

func (rf *Raft) sendAppendEntries(server int, args AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) beLeader() {
	rf.state = Leader
}

func (rf *Raft) beFollower(newTerm int) {
	rf.currentTerm = newTerm
	rf.state = Follower
}

func (rf *Raft) beCandidate() {
	rf.state = Candidate
}

func (rf *Raft) sendAppendLog() {
	args := AppendEntriesArgs{
		rf.currentTerm,
		rf.me,
	}
	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}

		go func(idx int) {
			reply := &AppendEntriesReply{}
			ret := rf.sendAppendEntries(idx, args, reply)
			if ret {

			}
		}(i)
	}
}

func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	// Your initialization code here.
	rf.state = Follower
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	heartbeatTime := time.Duration(100) * time.Millisecond

	go func() {
		for {
			electionTime := time.Duration(rand.Intn(200)+300) * time.Millisecond
			switch rf.state {
			case Follower, Candidate:
				time.Sleep(electionTime)
				rf.startElection()
			case Leader:
				time.Sleep(heartbeatTime)
				rf.sendAppendLog()
			}
		}
	}()
	return rf
}
```
```go
go test -run Ini
```
ok，现在执行，代码已经可以通过第一个test的了

虽然这里通过了，但是由于test的设计问题，一旦出现leader就会成功返回，而我们需要的是能够稳定存在的leader，希望heartbeat能够正常工作，但可以看到，即使成为了Follower，仍旧会进行选举，因此我们需要再加判断, 另外，投完票之后就不能再给其他人投票了，因此需要记录票投给谁了

## reElection

接下来我们看一下第二个test

```go
func TestReElection(t *testing.T) {
	servers := 3
	cfg := make_config(t, servers, false)
	defer cfg.cleanup()

	fmt.Printf("Test: election after network failure ...\n")

	leader1 := cfg.checkOneLeader()

	// if the leader disconnects, a new one should be elected.
	cfg.disconnect(leader1)
	cfg.checkOneLeader()

	// if the old leader rejoins, that shouldn't
	// disturb the old leader.
	cfg.connect(leader1)
	leader2 := cfg.checkOneLeader()

	// if there's no quorum, no leader should
	// be elected.
	cfg.disconnect(leader2)
	cfg.disconnect((leader2 + 1) % servers)
	time.Sleep(2 * RaftElectionTimeout)
	cfg.checkNoLeader()

	// if a quorum arises, it should elect a leader.
	cfg.connect((leader2 + 1) % servers)
	cfg.checkOneLeader()

	// re-join of last node shouldn't prevent leader from existing.
	cfg.connect(leader2)
	cfg.checkOneLeader()

	fmt.Printf("  ... Passed\n")
}
```

在上面代码中，出现了4种可能会出现的failure:
1. Leader失联，需要重新选举
2. Leader重新连接，不影响当前的Leader
3. 人数不够的话，不可以选出Leader
4. 如果人数又够的话，重新选举

首先看下disconnect之后会有什么反应：

```go
func (cfg *config) disconnect(i int) {
	// fmt.Printf("disconnect(%d)\n", i)

	cfg.connected[i] = false

	// outgoing ClientEnds
	for j := 0; j < cfg.n; j++ {
		if cfg.endnames[i] != nil {
			endname := cfg.endnames[i][j]
			cfg.net.Enable(endname, false)
		}
	}

	// incoming ClientEnds
	for j := 0; j < cfg.n; j++ {
		if cfg.endnames[j] != nil {
			endname := cfg.endnames[j][i]
			cfg.net.Enable(endname, false)
		}
	}
}
```

```go
// enable/disable a ClientEnd.
func (rn *Network) Enable(endname interface{}, enabled bool) {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	rn.enabled[endname] = enabled
}
```
cfg.net.Enable(endname, false)方法会使得labrpc下enabled为false，因此发送过去的信息无法被接收，其实就相当于停止发送heartbeat

