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

# Task I Implement leader election and heartbeats


