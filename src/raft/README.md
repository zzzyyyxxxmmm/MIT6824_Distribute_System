# BackGround

Thesis Link : [link](http://nil.csail.mit.edu/6.824/2016/papers/raft-extended.pdf)

## Consensus algorithm
Keeping the replicated log consistent is the job of the consensus algorithm.

Consensus algorithms for practical systems typically
have the following properties:
* They ensure safety (never returning an incorrect result) under all non-Byzantine conditions, including
network delays, partitions, and packet loss, duplication, and reordering.
* They are fully functional (available) as long as any
majority of the servers are operational and can communicate with each other and with clients. Thus, a
typical cluster of five servers can tolerate the failure
of any two servers. Servers are assumed to fail by
stopping; they may later recover from state on stable
storage and rejoin the cluster.
* They do not depend on timing to ensure the consis

# Raft 
Raft implements consensus by first electing a distinguished leader, then giving the leader complete responsibility for managing the replicated log. The leader accepts
log entries from clients, replicates them on other servers,
and tells servers when it is safe to apply log entries to
their state machines. Having a leader simplifies the management of the replicated log. For example, the leader can
decide where to place new entries in the log without consulting other servers, and data flows in a simple fashion
from the leader to other servers. A leader can fail or become disconnected from the other servers, in which case
a new leader is elected.

Given the leader approach, Raft decomposes the consensus problem into three relatively independent subproblems, which are discussed in the subsections that follow:
* **Leader election**: a new leader must be chosen when
an existing leader fails.
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

## Election
先描述一下整体流程:(写给自己温习的, 如果想要学习raft, 还是得读论文)
1. 所有server开始进入follower状态, 某个节点超时后进入candidate状态, 给自己投一票, term+1, 并且发送requestVote给所有其他节点, 其他节点如果没有投票,
并且term比candidate的term小, 就会给这个candidate投票, candiate如果收到大多数人的投票后就会进入leader状态.
2. leader开始周期性发送heartbeat给所有其他节点, 其他节点如果是candidate状态并且term比leader的term小, 那么该candidate进入follower状态, 每次follower收到heartbeat,
就会通过channel refresh 自己的时间, 以防进入election.
这里解释两种情况: (1) 假如leader断开了连接, 这个leader依旧会发送heartbeat, 并且保持leader状态. 其他server由于长时间收不到heartbeat就会进入1种的选举状态, 
假设这个时候leader又重新连接, 由于旧leader的term比新leader term小, 因此旧leader会变成follower. (2) 加入某个单独节点断开连接, 这个节点就会进入选举状态, 
但是该节点收不到其他节点的投票, 因此会不停进入election, term会一直增加.

## Log Replicate
我们的要求是, client发送log到我们的server, 如果我们server通知client这个log被执行了, 那么所有server都应该保证这个log是被执行的, 并且顺序是相同的. server会发送给每个server他需要添加的log, 
然后返回success给leader, leader觉得大部分节点都更新了, 就会自己更新并且通知client. 同时更新自己的commit信息并发送给server, server接收到commit信息后会commit. 这样leader和follower都commit了.

1. leader里持有nextIndex[i], 这个index表示第i个节点应该开始更新的地方, 同时会发送这个index的前一个log信息, 观察是否一致, 以保证log是一致的, 如果不一致则nextindex--, 总之第一个log是我们约定的, 一定相同的,
最差的情况就是, server的log和leader的log完全不同, 但server的log一定不会变, 所有follower的log都要和server的log同步

## snapshot
就是每次heartbeat的时候让server保存自己的状态, 重启的时候读取. 描述的挺复杂, 但实现起来随便就过了test

# 感想
写了一个星期, TestFigure8Unreliable没过, 测试的应该是网络延迟的情况, 比较难debug, 目前还没发现到底出了什么问题, 其实还是要从头开始写, 
细节情况很多, 论文里的figure2很重要, 每句话都是一个case, 细细体会. 



