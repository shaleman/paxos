# paxos library 
Paxos library provides a multi paxos implementation that can be used in variety of scenarios like:
- Distributed lock service
- Distributed consistent database
- Distributed embeded KV store

This paxos implentation provides a consensus based consistent log replication. 
Though this is a paxos implementation, its more closer to Raft algorithm in many aspects. 
It is less opinionated than Raft in Paxos implementation.

### Roles
Nodes in paxos can be in in leader, candidate or follower roles.
 - Leader: Leader is the distinguished proposer, distingushed listener and an acceptor in Paxos terms. Leader is responsible for adding new entries into the log and replicating it. Clients need to talk to the leader to make any changes to replicated log.
 - Candidate: candidate is a transient role for electing a leader. 
 - Follower: followers are responsible for accepting replication messages from the leader and voting for new leader. They are acceptors in paxos terms.
 - Rookie: When a new node joins a cluster, its in rookie state till it has synced all state from current leader.
 
### Messages

Prepare: Prepare message is sent by a candidate to request votes to become leader for a term. 

Extend: Extend message is used by the leader to extend his leader lease. Effectively this is used as a heartbeat message from the leader.

AcceptLog: AcceptLog message is used to append a set of log entries into replicated log. Unlike Raft, heartbeat is decoupled from accept log message so that leader can send as many acceptlog message as it chooses.

JoinNode: Join node message is sent by the leader to add a new node into cluster. This increases the size of the quorum.

LeaveNode: Leave node message is sent by the leader to remove a node from cluster. This decreases the size of the quorum.

SyncLog: Sync log message is used to sync replicated log from leader to newly joining node.

SyncSnapshot: Sync snapshot message is used to sync current snapshot from leader to newly joining node

### Leader election
When a cluster has no leader, all nodes enter the Candidate state, votes for itself and send a `Prepare` message to all other nodes. 
It sets the term id to currentTerm + 1 or 0 if currentTerm is 0.
If a candidate fails to get enough votes in a term, it backs off exponentially to a random number of terms.

Receivers of `Prepare` message vote for only node in a term, if it voted for itself or any other node in the term, it rejects the message.
Receivers of `prepare` message will not vote for a candidate whose term is lower than or equal to receiver's own currentTerm or lastLogIdx is lower than candidate's lastLogIdx.
This ensures a candidate who's replicated log behind majority of the nodes will not become new leader and overwrite previously committed log entry.

Once a leader has been elected successfully, it keeps extending its term using `Extend` message before its term expires. Generally, this is done half way thru the term to account for any clock drift.
No new leader will be elected before leader's term expires. 

### Log replication
Leader is solely responsible for appending entries into replicated log. Clients send requests to append entries to the leader.
Leader adds the entry to the log at currLogIdx + 1 entry, commits the log to persistent database and send `AcceptLog` message to all followers.
Once majority of followers have responded back to `AcceptLog` message, it sends an ack to the client.

### Adding new node to the cluster

