# paxos library 
Paxos library provides a multi paxos implementation that can be used in variety of scenarios like:
- Distributed lock and metadata service
- Distributed consistent database
- Replicated embeded KV store
- Replicated state machines

This paxos implentation provides a consensus based consistent log replication. 
Though this is a paxos implementation, its more closer to Raft algorithm in many aspects. 
It is less opinionated than Raft in Paxos implementation. Main reason for creating this library is most of the Raft implementations are ill suited for building high performance distributed consistent databases. This library tries to be high performance paxos implementation for high throughput applications
