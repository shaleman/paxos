// RPC transport for Paxos messages

package paxos

import (
	"errors"
	"net"
	"time"

	"golang.org/x/net/context"

	log "github.com/Sirupsen/logrus"
	"github.com/golang/glog"
	"github.com/shaleman/paxos/paxospb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/grpclog"
	gpt "google.golang.org/grpc/transport"
)

type Transport interface {
	Start()
	Stop()
	AddPeer(peerUrl string) error
	RemovePeer(peerUrl string) error
	SendPrepare(peerUrl string, req *paxospb.PrepareReq) (*paxospb.PrepareResp, error)
	SendExtend(peerUrl string, req *paxospb.ExtendReq) (*paxospb.ExtendResp, error)
	SendAcceptLog(peerUrl string, req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error)
}

type TransportCallback interface {
	NodeError(nodeUrl string)
	Prepare(req *paxospb.PrepareReq) (*paxospb.PrepareResp, error)
	Extend(req *paxospb.ExtendReq) (*paxospb.ExtendResp, error)
	AcceptLog(req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error)
	AddMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error)
	DelMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error)
}

type GrpcTransport struct {
	// RPC server
	grpcServer *grpc.Server

	// Url where server is listening
	listenUrl string

	// Message handler
	handler TransportCallback

	// map of RPC clients
	clients map[string]paxospb.PaxosClient
}

// NewGrpcTransport creates a new transport
// FIXME: add TLS and configurable timeout
func NewGrpcTransport(handler TransportCallback, listenUrl string) (Transport, error) {
	// init a grpc transport object
	trans := GrpcTransport{}
	trans.clients = make(map[string]paxospb.PaxosClient)
	trans.handler = handler
	trans.listenUrl = listenUrl

	// start the grpc server
	trans.Start()

	// stop those annoying grpc Logs
	grpclog.SetLogger(&glogger{})

	return &trans, nil
}

// Start starts the grpc server
func (trans *GrpcTransport) Start() {
	// Start a listener
	lis, err := net.Listen("tcp", trans.listenUrl)
	if err != nil {
		log.Fatalf("failed to listen to %s: Err %v", trans.listenUrl, err)
	}

	// start new grpc server
	server := grpc.NewServer()
	trans.grpcServer = server
	paxospb.RegisterPaxosServer(server, trans)
	go server.Serve(lis)

	log.Infof("gRpc serverListening on %s", trans.listenUrl)
}

// Stop stops grpc server
func (trans *GrpcTransport) Stop() {
	trans.grpcServer.Stop()
	trans.grpcServer = nil
	log.Infof("Stopped gRpc server %s", trans.listenUrl)
}

// AddPeer connects to a peer
func (trans *GrpcTransport) AddPeer(peerUrl string) error {
	// Set up a connection to the server.
	conn, err := grpc.Dial(peerUrl, grpc.WithInsecure(), grpc.WithTimeout(time.Second*3))
	if err != nil {
		log.Errorf("could not connect to %s. Err: %v", peerUrl, err)
		return err
	}

	log.Infof("Node %s Connected to %s", trans.listenUrl, peerUrl)

	// create a client
	c := paxospb.NewPaxosClient(conn)
	trans.clients[peerUrl] = c

	return nil
}

// RemovePeer disconnects from a peer
func (trans *GrpcTransport) RemovePeer(peerUrl string) error {
	_, ok := trans.clients[peerUrl]
	if !ok {
		log.Errorf("Could not find peer %s", peerUrl)
		return errors.New("Peer not found")
	}

	delete(trans.clients, peerUrl)

	return nil
}

// handleRpcError Handle an RPC error
func (trans *GrpcTransport) handleRpcError(peerUrl string, err error) {
	if (err == gpt.ErrConnClosing) || (grpc.ErrorDesc(err) == grpc.ErrClientConnClosing.Error()) {
		log.Infof("Node %s disconnected. Err: %v", peerUrl, err)

		// remove the Peer
		trans.RemovePeer(peerUrl)

		// make callbacks
		trans.handler.NodeError(peerUrl)
	}
}

// SendPrepare sends prepare message to a peer
func (trans *GrpcTransport) SendPrepare(peerUrl string, req *paxospb.PrepareReq) (*paxospb.PrepareResp, error) {
	// find the client
	client, ok := trans.clients[peerUrl]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerUrl)
		return nil, errors.New("Client not found")
	}

	// RPC call
	resp, err := client.Prepare(context.Background(), req)
	if err != nil {
		log.Errorf("Error during Prepare rpc call to %s: %v", peerUrl, err)
		trans.handleRpcError(peerUrl, err)
		return resp, err
	}

	return resp, nil
}

// SendExtend sends extend message to a peer
func (trans *GrpcTransport) SendExtend(peerUrl string, req *paxospb.ExtendReq) (*paxospb.ExtendResp, error) {
	// find the client
	client, ok := trans.clients[peerUrl]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerUrl)
		return nil, errors.New("Client not found")
	}

	// RPC call
	resp, err := client.Extend(context.Background(), req)
	if err != nil {
		log.Errorf("Error during Extend rpc call to %s: %v", peerUrl, err)
		trans.handleRpcError(peerUrl, err)
		return resp, err
	}

	return resp, nil
}

// SendAcceptLog sends accept message to a peer
func (trans *GrpcTransport) SendAcceptLog(peerUrl string, req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error) {
	// find the client
	client, ok := trans.clients[peerUrl]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerUrl)
		return nil, errors.New("Client not found")
	}

	// RPC call
	resp, err := client.AcceptLog(context.Background(), req)
	if err != nil {
		log.Errorf("Error during AcceptLog rpc call to %s: %v", peerUrl, err)
		trans.handleRpcError(peerUrl, err)
		return resp, err
	}

	return resp, nil
}

// Prepare handles prepare message
func (trans *GrpcTransport) Prepare(ctx context.Context, req *paxospb.PrepareReq) (*paxospb.PrepareResp, error) {
	return trans.handler.Prepare(req)
}

// Extend handles extend message
func (trans *GrpcTransport) Extend(ctx context.Context, req *paxospb.ExtendReq) (*paxospb.ExtendResp, error) {
	return trans.handler.Extend(req)
}

// AcceptLog handles AcceptLog message
func (trans *GrpcTransport) AcceptLog(ctx context.Context, req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error) {
	return trans.handler.AcceptLog(req)
}

func (trans *GrpcTransport) AddMember(ctx context.Context, req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return trans.handler.AddMember(req)
}

func (trans *GrpcTransport) DelMember(ctx context.Context, req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return trans.handler.DelMember(req)
}

func (trans *GrpcTransport) RequestSync(req *paxospb.SyncReq, stream paxospb.Paxos_RequestSyncServer) error {
	return nil
}

// Dummy logger to prevent unnecesary grpc logs
type glogger struct{}

func (g *glogger) Fatal(args ...interface{}) {
	glog.Fatal(args...)
}

func (g *glogger) Fatalf(format string, args ...interface{}) {
	glog.Fatalf(format, args...)
}

func (g *glogger) Fatalln(args ...interface{}) {
	glog.Fatalln(args...)
}

func (g *glogger) Print(args ...interface{}) {
	// glog.Info(args...)
}

func (g *glogger) Printf(format string, args ...interface{}) {
	// glog.Infof(format, args...)
}

func (g *glogger) Println(args ...interface{}) {
	// glog.Infoln(args...)
}
