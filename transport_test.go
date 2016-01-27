// Test paxos transport

package paxos

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	. "gopkg.in/check.v1"

	log "github.com/Sirupsen/logrus"
	"github.com/shaleman/paxos/paxospb"
)

const basePort = 5001
const streamCount = 1000

type cluster struct {
	transports      map[string]Transport
	numPrepareReq   int
	numExtendReq    int
	numAcceptLogReq int
	numNodeError    int
}

func (cl *cluster) NodeError(nodelURL string) {
	cl.numNodeError++
	log.Infof("Received node error for %s", nodelURL)
}

func (cl *cluster) Prepare(req *paxospb.PrepareReq) (*paxospb.PrepareResp, error) {
	cl.numPrepareReq++
	log.Debugf("Received Prepare rpc call from: %s", req.RequesterId)
	resp := paxospb.PrepareResp{
		VotedFor:    "test",
		CurrentTerm: 1,
		LastLogIdx:  1,
	}
	return &resp, nil
}
func (cl *cluster) Extend(req *paxospb.ExtendReq) (*paxospb.ExtendResp, error) {
	cl.numExtendReq++
	return nil, errors.New("Testing RPC error")
}
func (cl *cluster) AcceptLog(req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error) {
	cl.numAcceptLogReq++
	return &paxospb.AcceptLogResp{}, nil
}

func (cl *cluster) RequestSync(req *paxospb.SyncReq, stream paxospb.Paxos_RequestSyncServer) error {
	for i := 0; i < streamCount; i++ {
		entry := paxospb.SyncEntry{
			EntryType: paxospb.EntryType_LogType,
			LogEntry: &paxospb.LogEntry{
				Term:   uint64(i),
				LogIdx: uint64(i),
			},
		}
		if err := stream.Send(&entry); err != nil {
			return err
		}
	}
	return nil
}

func (cl *cluster) AddMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return &paxospb.MemberUpdateResp{}, nil
}

func (cl *cluster) DelMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return &paxospb.MemberUpdateResp{}, nil
}

// createFakeCluster creates a cluster of nodes talking to each other usint transport
func createFakeCluster(numNodes int) (*cluster, error) {
	c := cluster{
		transports: make(map[string]Transport),
	}

	// Create all transports
	for i := 0; i < numNodes; i++ {
		nodelURL := fmt.Sprintf("localhost:%d", (basePort + i))

		// create the transport
		trans, err := NewGrpcTransport(&c, nodelURL)
		if err != nil {
			log.Errorf("Failed to create transport %s. Err: %v", nodelURL, err)
			return nil, err
		}

		c.transports[nodelURL] = trans
	}

	// Add each other nodes as peers
	for i := 0; i < numNodes; i++ {
		nodelURL := fmt.Sprintf("localhost:%d", (basePort + i))

		for j := 0; j < numNodes; j++ {
			peerURL := fmt.Sprintf("localhost:%d", (basePort + j))
			if j != i {
				err := c.transports[nodelURL].AddPeer(peerURL)
				if err != nil {
					log.Errorf("Error adding peer %s to %s. Err: %v", peerURL, nodelURL, err)
					return nil, err
				}
			}
		}
	}

	return &c, nil
}

// makeFullMeshRPCCalls make full mesh RPC calls within the cluster
func (cl *cluster) makeFullMeshRPCCalls() error {
	// Make full mesh rpc calls
	for nodelURL, trans := range cl.transports {
		for peerURL := range cl.transports {
			if nodelURL != peerURL {
				_, err := trans.SendPrepare(peerURL, &paxospb.PrepareReq{
					RequesterId: nodelURL,
					Term:        1,
					LastLogIdx:  1,
					LeaseTime:   100,
				})
				if err != nil {
					log.Errorf("SendPrepare Rpc call err from %s to %s. Err: %v", nodelURL, peerURL, err)
					return err
				}
			}
		}
	}

	return nil
}

func (cl *cluster) destroy() error {
	for _, trans := range cl.transports {
		// stop the server
		trans.Stop()
	}

	return nil
}

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type TransportSuite struct{}

var _ = Suite(&TransportSuite{})

func (s *TransportSuite) TestTransportRpcs(c *C) {
	numNodes := 5
	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	// Make full mesh rpc calls
	c.Assert(cl.makeFullMeshRPCCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))
	c.Check(cl.numAcceptLogReq, Equals, 0)
	c.Check(cl.numExtendReq, Equals, 0)
}

func (s *TransportSuite) TestTransportDisconnectConnect(c *C) {
	numNodes := 5
	numIter := 3

	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	// Make full mesh rpc calls
	c.Assert(cl.makeFullMeshRPCCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))

	// Repeatedly disconnect the server and make sure we connect back
	for i := 0; i < numIter; i++ {
		cl.numPrepareReq = 0

		for _, trans := range cl.transports {
			// stop the server
			trans.Stop()

			time.Sleep(time.Millisecond * 10)

			// restart the server
			trans.Start()
		}

		// Make full mesh rpc calls
		c.Assert(cl.makeFullMeshRPCCalls(), IsNil)

		// verify rpc counts are correct
		c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))
	}
}

func (s *TransportSuite) TestTransportDown(c *C) {
	numNodes := 3

	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	// Make full mesh rpc calls
	c.Assert(cl.makeFullMeshRPCCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))

	cl.numPrepareReq = 0

	for nodelURL, trans := range cl.transports {
		// stop the server
		trans.Stop()

		time.Sleep(time.Millisecond * 10)

		// Try to make RPC calls from all peers to this node and verify it fails
		for peerURL, peer := range cl.transports {
			if nodelURL != peerURL {
				_, err := peer.SendPrepare(nodelURL, &paxospb.PrepareReq{
					RequesterId: peerURL,
					Term:        1,
					LastLogIdx:  1,
					LeaseTime:   100,
				})
				c.Assert(err, NotNil)
			}
		}

		time.Sleep(time.Millisecond * 10)

		// restart the server
		trans.Start()

		// add the peer back
		for peerURL, node := range cl.transports {
			if peerURL != nodelURL {
				node.AddPeer(nodelURL)
			}
		}
	}

	// Make sure we received node error
	c.Check(cl.numNodeError, Equals, (numNodes * (numNodes - 1)))

	// Make full mesh rpc calls
	c.Assert(cl.makeFullMeshRPCCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))
}

func (s *TransportSuite) TestTransportRpcError(c *C) {
	numNodes := 5

	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	// Make full mesh rpc calls
	c.Assert(cl.makeFullMeshRPCCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))

	// Make full mesh rpc calls
	for nodelURL, trans := range cl.transports {
		for peerURL := range cl.transports {
			if nodelURL != peerURL {
				_, err := trans.SendExtend(peerURL, &paxospb.ExtendReq{
					RequesterId: nodelURL,
					Term:        1,
					LeaseTime:   100,
				})
				c.Assert(err, NotNil)
			}
		}
	}
}

func (s *TransportSuite) TestTransportRpcStream(c *C) {
	numNodes := 5

	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	// Make full mesh stream rpc calls
	for nodelURL, trans := range cl.transports {
		for peerURL := range cl.transports {
			if nodelURL != peerURL {
				stream, err := trans.SendRequestSync(peerURL, &paxospb.SyncReq{
					MemberId: nodelURL,
				})
				c.Assert(err, IsNil)
				rxStreamCount := 0
				for {
					entry, err := stream.Recv()
					if err == io.EOF {
						break
					}
					c.Assert(err, IsNil)
					c.Check(entry.EntryType, Equals, paxospb.EntryType_LogType)
					c.Check(entry.LogEntry.Term, Equals, uint64(rxStreamCount))
					c.Check(entry.LogEntry.LogIdx, Equals, uint64(rxStreamCount))
					rxStreamCount++
				}
				c.Check(rxStreamCount, Equals, streamCount)
				log.Infof("Streaming RPC Received %d entries, expected %d", rxStreamCount, streamCount)
			}
		}
	}
}

func (s *TransportSuite) BenchmarkTransportRpcCall(c *C) {
	numNodes := 2

	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	serverURL := fmt.Sprintf("localhost:%d", (basePort + 1))
	clientURL := fmt.Sprintf("localhost:%d", (basePort + 0))
	client := cl.transports[clientURL]

	// run benchmark
	for n := 0; n < c.N; n++ {
		_, err := client.SendPrepare(serverURL, &paxospb.PrepareReq{
			RequesterId: "test",
			Term:        1,
			LastLogIdx:  1,
			LeaseTime:   100,
		})
		if err != nil {
			c.Fatalf("Failed to make RPC call from %s to %s. Err: %v", serverURL, clientURL, err)
		}
	}
}
