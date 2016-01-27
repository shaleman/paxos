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

// Transport interface implemented by all transports
type Transport interface {
	Start()
	Stop()
	AddPeer(peerURL string) error
	RemovePeer(peerURL string) error
	SendPrepare(peerURL string, req *paxospb.PrepareReq) (*paxospb.PrepareResp, error)
	SendExtend(peerURL string, req *paxospb.ExtendReq) (*paxospb.ExtendResp, error)
	SendAcceptLog(peerURL string, req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error)
	SendRequestSync(peerURL string, req *paxospb.SyncReq) (paxospb.Paxos_RequestSyncClient, error)
}

// TransportCallback interface needs to be implemented by paxos
// Transports call these functions when thry receive an incoming RPC message
type TransportCallback interface {
	NodeError(nodeURL string)
	Prepare(req *paxospb.PrepareReq) (*paxospb.PrepareResp, error)
	Extend(req *paxospb.ExtendReq) (*paxospb.ExtendResp, error)
	AcceptLog(req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error)
	AddMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error)
	DelMember(req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error)
	RequestSync(req *paxospb.SyncReq, stream paxospb.Paxos_RequestSyncServer) error
}

// GrpcTransport implements gRpc based transport
// This is based on google's gRpc. see www.grpc.io for more details
type GrpcTransport struct {
	// RPC server
	grpcServer *grpc.Server

	// Url where server is listening
	listenURL string

	// Message handler
	handler TransportCallback

	// map of RPC clients
	clients map[string]paxospb.PaxosClient
}

// NewGrpcTransport creates a new transport
// FIXME: add TLS and configurable timeout
func NewGrpcTransport(handler TransportCallback, listenURL string) (Transport, error) {
	// init a grpc transport object
	trans := GrpcTransport{}
	trans.clients = make(map[string]paxospb.PaxosClient)
	trans.handler = handler
	trans.listenURL = listenURL

	// start the grpc server
	trans.Start()

	// stop those annoying grpc Logs
	grpclog.SetLogger(&glogger{})

	return &trans, nil
}

// Start starts the grpc server
func (trans *GrpcTransport) Start() {
	// Start a listener
	lis, err := net.Listen("tcp", trans.listenURL)
	if err != nil {
		log.Fatalf("failed to listen to %s: Err %v", trans.listenURL, err)
	}

	// start new grpc server
	server := grpc.NewServer()
	trans.grpcServer = server
	paxospb.RegisterPaxosServer(server, trans)
	go server.Serve(lis)

	log.Infof("gRpc serverListening on %s", trans.listenURL)
}

// Stop stops grpc server
func (trans *GrpcTransport) Stop() {
	trans.grpcServer.Stop()
	trans.grpcServer = nil
	log.Infof("Stopped gRpc server %s", trans.listenURL)
}

// AddPeer connects to a peer
func (trans *GrpcTransport) AddPeer(peerURL string) error {
	// Set up a connection to the server.
	conn, err := grpc.Dial(peerURL, grpc.WithInsecure(), grpc.WithTimeout(time.Second*3))
	if err != nil {
		log.Errorf("could not connect to %s. Err: %v", peerURL, err)
		return err
	}

	log.Infof("Node %s Connected to %s", trans.listenURL, peerURL)

	// create a client
	c := paxospb.NewPaxosClient(conn)
	trans.clients[peerURL] = c

	return nil
}

// RemovePeer disconnects from a peer
func (trans *GrpcTransport) RemovePeer(peerURL string) error {
	_, ok := trans.clients[peerURL]
	if !ok {
		log.Errorf("Could not find peer %s", peerURL)
		return errors.New("Peer not found")
	}

	delete(trans.clients, peerURL)

	return nil
}

// handleRPCError Handle an RPC error
func (trans *GrpcTransport) handleRPCError(peerURL string, err error) {
	if (err == gpt.ErrConnClosing) || (grpc.ErrorDesc(err) == grpc.ErrClientConnClosing.Error()) {
		log.Infof("Node %s disconnected. Err: %v", peerURL, err)

		// remove the Peer
		trans.RemovePeer(peerURL)

		// make callbacks
		trans.handler.NodeError(peerURL)
	}
}

// SendPrepare sends prepare message to a peer
func (trans *GrpcTransport) SendPrepare(peerURL string, req *paxospb.PrepareReq) (*paxospb.PrepareResp, error) {
	// find the client
	client, ok := trans.clients[peerURL]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerURL)
		return nil, errors.New("Client not found")
	}

	// RPC call
	resp, err := client.Prepare(context.Background(), req)
	if err != nil {
		log.Errorf("Error during Prepare rpc call to %s: %v", peerURL, err)
		trans.handleRPCError(peerURL, err)
		return resp, err
	}

	return resp, nil
}

// SendExtend sends extend message to a peer
func (trans *GrpcTransport) SendExtend(peerURL string, req *paxospb.ExtendReq) (*paxospb.ExtendResp, error) {
	// find the client
	client, ok := trans.clients[peerURL]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerURL)
		return nil, errors.New("Client not found")
	}

	// RPC call
	resp, err := client.Extend(context.Background(), req)
	if err != nil {
		log.Errorf("Error during Extend rpc call to %s: %v", peerURL, err)
		trans.handleRPCError(peerURL, err)
		return resp, err
	}

	return resp, nil
}

// SendAcceptLog sends accept message to a peer
func (trans *GrpcTransport) SendAcceptLog(peerURL string, req *paxospb.AcceptLogReq) (*paxospb.AcceptLogResp, error) {
	// find the client
	client, ok := trans.clients[peerURL]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerURL)
		return nil, errors.New("Client not found")
	}

	// RPC call
	resp, err := client.AcceptLog(context.Background(), req)
	if err != nil {
		log.Errorf("Error during AcceptLog rpc call to %s: %v", peerURL, err)
		trans.handleRPCError(peerURL, err)
		return resp, err
	}

	return resp, nil
}

// SendRequestSync sends request sync message
func (trans *GrpcTransport) SendRequestSync(peerURL string, req *paxospb.SyncReq) (paxospb.Paxos_RequestSyncClient, error) {
	// find the client
	client, ok := trans.clients[peerURL]
	if !ok {
		log.Errorf("Could not find the clinet %s", peerURL)
		return nil, errors.New("Client not found")
	}

	// RPC call
	stream, err := client.RequestSync(context.Background(), req)
	if err != nil {
		log.Errorf("Error during AcceptLog rpc call to %s: %v", peerURL, err)
		trans.handleRPCError(peerURL, err)
		return stream, err
	}

	return stream, nil
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

// AddMember handles add member request
func (trans *GrpcTransport) AddMember(ctx context.Context, req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return trans.handler.AddMember(req)
}

// DelMember handles delete member request
func (trans *GrpcTransport) DelMember(ctx context.Context, req *paxospb.MemberUpdate) (*paxospb.MemberUpdateResp, error) {
	return trans.handler.DelMember(req)
}

// RequestSync handles request sync message
func (trans *GrpcTransport) RequestSync(req *paxospb.SyncReq, stream paxospb.Paxos_RequestSyncServer) error {
	return trans.handler.RequestSync(req, stream)
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
