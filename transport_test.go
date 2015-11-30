// Test paxos transport

package paxos

import (
	"fmt"
	"testing"
	"time"

	. "gopkg.in/check.v1"

	log "github.com/Sirupsen/logrus"
	"github.com/shaleman/paxos/paxospb"
)

const basePort = 5001

type cluster struct {
	transports      map[string]Transport
	numPrepareReq   int
	numExtendReq    int
	numAcceptLogReq int
	numNodeError    int
}

func (c *cluster) NodeError(nodeUrl string) {
	c.numNodeError++
	log.Infof("Received node error for %s", nodeUrl)
}

func (c *cluster) Prepare(req *paxospb.PrepareReq) (*paxospb.PrepareResp, error) {
	c.numPrepareReq++
	log.Debugf("Received Prepare rpc call from: %s", req.RequesterId)
	resp := paxospb.PrepareResp{
		VotedFor:    "test",
		CurrentTerm: 1,
		LastLogIdx:  1,
	}
	return &resp, nil
}
func (c *cluster) Extend(req *paxospb.ExtendReq) (*paxospb.ExtendResp, error) {
	c.numExtendReq++
	return &paxospb.ExtendResp{}, nil
}
func (c *cluster) AcceptLog(req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error) {
	c.numAcceptLogReq++
	return &paxospb.AcceptLogResp{}, nil
}

func (c *cluster) AddMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return &paxospb.MemberUpdateResp{}, nil
}

func (c *cluster) DelMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return &paxospb.MemberUpdateResp{}, nil
}

// createFakeCluster creates a cluster of nodes talking to each other usint transport
func createFakeCluster(numNodes int) (*cluster, error) {
	c := cluster{
		transports: make(map[string]Transport),
	}

	// Create all transports
	for i := 0; i < numNodes; i++ {
		nodeUrl := fmt.Sprintf("localhost:%d", (basePort + i))

		// create the transport
		trans, err := NewGrpcTransport(&c, nodeUrl)
		if err != nil {
			log.Errorf("Failed to create transport %s. Err: %v", nodeUrl, err)
			return nil, err
		}

		c.transports[nodeUrl] = trans
	}

	// Add each other nodes as peers
	for i := 0; i < numNodes; i++ {
		nodeUrl := fmt.Sprintf("localhost:%d", (basePort + i))

		for j := 0; j < numNodes; j++ {
			peerUrl := fmt.Sprintf("localhost:%d", (basePort + j))
			if j != i {
				err := c.transports[nodeUrl].AddPeer(peerUrl)
				if err != nil {
					log.Errorf("Error adding peer %s to %s. Err: %v", peerUrl, nodeUrl, err)
					return nil, err
				}
			}
		}
	}

	return &c, nil
}

// makeFullMeshRpcCalls make full mesh RPC calls within the cluster
func (cl *cluster) makeFullMeshRpcCalls() error {
	// Make full mesh rpc calls
	for nodeUrl, trans := range cl.transports {
		for peerUrl, _ := range cl.transports {
			if nodeUrl != peerUrl {
				_, err := trans.SendPrepare(peerUrl, &paxospb.PrepareReq{
					RequesterId: nodeUrl,
					Term:        1,
					LastLogIdx:  1,
					LeaseTime:   100,
				})
				if err != nil {
					log.Errorf("SendPrepare Rpc call err from %s to %s. Err: %v", nodeUrl, peerUrl, err)
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
	c.Assert(cl.makeFullMeshRpcCalls(), IsNil)

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
	c.Assert(cl.makeFullMeshRpcCalls(), IsNil)

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
		c.Assert(cl.makeFullMeshRpcCalls(), IsNil)

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
	c.Assert(cl.makeFullMeshRpcCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))

	cl.numPrepareReq = 0

	for nodeUrl, trans := range cl.transports {
		// stop the server
		trans.Stop()

		time.Sleep(time.Millisecond * 10)

		// Try to make RPC calls from all peers to this node and verify it fails
		for peerUrl, peer := range cl.transports {
			if nodeUrl != peerUrl {
				_, err := peer.SendPrepare(nodeUrl, &paxospb.PrepareReq{
					RequesterId: peerUrl,
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
		for peerUrl, node := range cl.transports {
			if peerUrl != nodeUrl {
				node.AddPeer(nodeUrl)
			}
		}
	}

	// Make sure we received node error
	c.Check(cl.numNodeError, Equals, (numNodes * (numNodes - 1)))

	// Make full mesh rpc calls
	c.Assert(cl.makeFullMeshRpcCalls(), IsNil)

	// verify rpc counts are correct
	c.Check(cl.numPrepareReq, Equals, (numNodes * (numNodes - 1)))
}

func (s *TransportSuite) BenchmarkTransportRpcCall(c *C) {
	numNodes := 2

	cl, err := createFakeCluster(numNodes)
	c.Assert(err, IsNil)
	defer func() {
		cl.destroy()
		time.Sleep(time.Millisecond * 10)
	}()

	serverUrl := fmt.Sprintf("localhost:%d", (basePort + 1))
	clientUrl := fmt.Sprintf("localhost:%d", (basePort + 0))
	client := cl.transports[clientUrl]

	// run benchmark
	for n := 0; n < c.N; n++ {
		_, err := client.SendPrepare(serverUrl, &paxospb.PrepareReq{
			RequesterId: "test",
			Term:        1,
			LastLogIdx:  1,
			LeaseTime:   100,
		})
		if err != nil {
			c.Fatalf("Failed to make RPC call from %s to %s. Err: %v", serverUrl, clientUrl, err)
		}
	}
}
