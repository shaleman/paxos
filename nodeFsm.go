// This file implements the node state machine

package paxos

import (
	"math/rand"
	"sync"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/shaleman/paxos/paxospb"
)

// Config config params for paxos instance
type Config struct {
	TermDuration time.Duration // duration of each term
}

type nodeFsm struct {
	nodeID       string    // Unique identifier for the state
	nodeURL      string    // my URL
	currState    string    // current state
	votedInTerm  bool      // Already voted in this term
	lastContact  time.Time // Lats contact from the leader
	backoffIntvl int       // backoff interval for leader election

	conf *Config // Config params

	nodeMutex sync.Mutex // Global lock for node level state

	// channels used for communication
	peerReqChan chan *peerReq

	// preqChan    chan *paxospb.PrepareReq
	// prespChan   chan *paxospb.PrepareResp
	// ereqChan    chan *paxospb.ExtendReq
	// erespChan   chan *paxospb.ExtendResp
}

type peerResp struct {
	msgType     string
	err         error
	prepareResp *paxospb.PrepareResp
	extendResp  *paxospb.ExtendResp
}

type peerReq struct {
	msgType  string
	prepare  *paxospb.PrepareReq
	extend   *paxospb.ExtendReq
	respChan chan *peerResp
}

func (preq *peerReq) respond(resp *peerResp) {
	preq.respChan <- resp
}

// newNodeFsm creates a new node state machine
func newNodeFsm(nodeID, nodeURL string, conf *Config) (*nodeFsm, error) {
	node := new(nodeFsm)
	node.nodeID = nodeID
	node.nodeURL = nodeURL
	node.conf = conf
	node.currState = "Follower"

	// Run the node state machine
	go node.runNodeFsm()

	return node, nil
}

// GetState returns current state
func (node *nodeFsm) GetState() string {
	return node.currState
}

// SetState sets new state
func (node *nodeFsm) SetState(newState string) {
	node.nodeMutex.Lock()
	if node.currState != newState {
		log.Infof("Node %s transitioning from %s state to %s state", node.nodeURL, node.currState, newState)
	}
	node.currState = newState
	node.nodeMutex.Unlock()
}

// Loop forever running the node state machine
func (node *nodeFsm) runNodeFsm() {
	for {
		switch node.GetState() {
		case "Follower":
			node.runFollower()
		case "Candidate":
			node.runCandidate()
		case "Leader":
			node.runLeader()
		default:
			log.Fatalf("Unknown node state %s", node.GetState())
		}
	}
}

// runFollower runs the follower loop
func (node *nodeFsm) runFollower() {
	// start a timer to keep track of the term
	termTimer := randomTimeout(node.conf.TermDuration)

	// set current time as the last known contact from leader
	node.lastContact = time.Now()
	for {
		select {
		case preq := <-node.peerReqChan:
			node.handleReqMsg(preq)
		case tt := <-termTimer:
			// reset voted flag after each term
			node.votedInTerm = false

			// check if we heard from the leader in the last term
			if time.Now().Sub(node.lastContact) > node.conf.TermDuration {
				// Leader timed out. Transition to candidate state
				node.SetState("Candidate")

				// exit the follower loop
				return
			}

			// restart the timer
			termTimer = randomTimeout(node.conf.TermDuration)
		}
	}
}

// runCandidate runs candidate loop
func (node *nodeFsm) runCandidate() {
	// vote for self
	node.votedInTerm = true

	// send prepare request to all peers
	quorum := node.paxos.sendPrepare()
	if quorum {
		// We got the quorum. We are the new leader
		node.SetState("Leader")

		return
	}

	// Create a timer that fires once per term
	termTimer := randomTimeout(node.conf.TermDuration)

	for {
		select {
		case preq := <-node.peerReqChan:
			node.handleReqMsg(preq)
		case tt := <-termTimer:
			// clear voted flag
			node.votedInTerm = false

			// Exponentially backoff and send prepare req
			if node.backoffIntvl == 0 {
				// send prepare request to all peers
				quorum := node.paxos.sendPrepare()
				if quorum {
					// We got the quorum. We are the new leader
					node.SetState("Leader")

					return
				}

				// If leader election failed, backoff
				// FIXME: Exponentially backoff
				node.backoffIntvl = rand.Int63() % 10
			}

			// Decrement backoff interval
			node.backoffIntvl = node.backoffIntvl - 1
		}
	}
}
func (node *nodeFsm) runLeader() {
}

// handleReqMsg handles a messgae from peer
func (node *nodeFsm) handleReqMsg(preq *peerReq) {
	switch preq.msgType {
	case "Prepare":
		// handle prepare request
		resp := node.handlePrepare(preq.prepare)

		// build the response
		presp := peerResp{
			msgType: "Prepare",
			prepare: resp,
		}

		// send the response
		preq.respond(presp)
	case "Extend":
		// handle extend message
		resp := node.handleExtend()

		// build the response
		presp := peerResp{
			msgType: "Extend",
			extend:  resp,
		}

		// send the response
		preq.respond(presp)
	}
}

// Returns a jittered timer. timer is randomly jittered 33%
func randomTimeout(timeout time.Duration) <-chan time.Time {
	if timeout == 0 {
		return nil
	}
	extra := (time.Duration(rand.Int63()) % (timeout / 3))
	return time.After(timeout + extra)
}
