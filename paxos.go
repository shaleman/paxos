// Main Paxos APIs

package paxos

import (
	"log"

	"github.com/shaleman/paxos/paxospb"
)

// Paxos API
type Paxos interface {
	// Add a new member to paxos group
	MemberJoin(nodeID, nodeURL string) error

	// Delete a member from paxos group
	MemberLeave(nodeID string) error

	// Database took a snapshot
	Snapshot(term, logIdx uint64) error
}

// Database needs to be supported by database
type Database interface {
	// Paxos calls apply when a log entry can be applied to the database
	Apply(logEntries []*paxospb.LogEntry) error

	// Paxos calls GetSnapshot when it needs to sync snapshot to a peer node
	GetSnapshot() ([]*paxospb.LogEntry, error)
}

// LogStore interface needs to be supported by persistent log stores
type LogStore interface {
	AppendEntries(logEntries []*paxospb.LogEntry)
}

// paxosState maintains the state of the paxos instance
type paxosState struct {
	myNodeID  string // unique node id for this node
	listenURL string // addr/port to listen on

	transport Transport // RPC transport
	logStore  LogStore  // persistent store for log entries
	database  Database  // Database instance storing state
}

// PaxosConfig configuration for paxos instance
type PaxosConfig struct {
	listenURL string // addr/port to listen on
}

// NewPaxos creates a new paxos instance
func NewPaxos(listenURL string, logStore LogStore, database Database) (Paxos, error) {
	paxos := new(paxosState)
	paxos.logStore = logStore
	paxos.database = database

	// Create grpc transport
	transport, err := NewGrpcTransport(paxos, listenURL)
	if err != nil {
		log.Fatalf("Error initializing transport on %s. Err: %v", listenURL, err)
	}

	paxos.transport = transport

	return paxos, nil
}

func (px *paxosState) MemberJoin(nodeID, nodeURL string) error {
	return nil
}

func (px *paxosState) MemberLeave(nodeID string) error {
	return nil
}

func (px *paxosState) Snapshot(term, logIdx uint64) error {
	return nil
}

func (px *paxosState) NodeError(nodeURL string) {
}

func (px *paxosState) Prepare(req *paxospb.PrepareReq) (*paxospb.PrepareResp, error) {
	return nil, nil
}

func (px *paxosState) Extend(req *paxospb.ExtendReq) (*paxospb.ExtendResp, error) {
	return nil, nil
}

func (px *paxosState) AcceptLog(req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error) {
	return nil, nil
}

func (px *paxosState) AddMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return nil, nil
}

func (px *paxosState) DelMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return nil, nil
}

func (px *paxosState) RequestSync(req *paxospb.SyncReq, stream paxospb.Paxos_RequestSyncServer) error {
	return nil
}
