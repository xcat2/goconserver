package console

import (
	"context"
	"errors"
	"fmt"
	"github.com/chenglch/goconserver/common"
	pb "github.com/chenglch/goconserver/console/consolepb"
	net_context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
)

type ConsoleRPCServer struct {
	port   string
	host   string
	server *grpc.Server
}

func newConsoleRPCServer() *ConsoleRPCServer {
	return &ConsoleRPCServer{port: serverConfig.Console.RPCPort, host: serverConfig.Global.Host}
}

func (s *ConsoleRPCServer) ShowNode(ctx net_context.Context, rpcNode *pb.NodeName) (*pb.Node, error) {
	plog.Debug("Receive the RPC call ShowNode")
	nodeManager.RWlock.RLock()
	if !nodeManager.Exists(rpcNode.Name) {
		nodeManager.RWlock.RUnlock()
		return nil, errors.New(fmt.Sprintf("Could not find node %s on %s", rpcNode.Name, serverConfig.Global.Host))
	}
	node := nodeManager.Nodes[rpcNode.Name]
	retNode := pb.Node{Name: node.StorageNode.Name,
		Driver:   node.StorageNode.Driver,
		Params:   node.StorageNode.Params,
		Ondemand: node.StorageNode.Ondemand,
		Status:   int32(node.status)}
	nodeManager.RWlock.RUnlock()
	return &retNode, nil
}

func (s *ConsoleRPCServer) SetConsoleState(ctx net_context.Context, pbNodesStae *pb.NodesState) (*pb.Result, error) {
	plog.Debug("Receive the RPC call SetConsoleState")
	nodeManager.RWlock.RLock()
	names := make([]string, 0, len(pbNodesStae.Names))
	for _, name := range pbNodesStae.Names {
		if !nodeManager.Exists(name) {
			plog.ErrorNode(name, fmt.Sprintf("Could not find node on %s", serverConfig.Global.Host))
			continue
		}
		names = append(names, name)
	}
	nodeManager.RWlock.RUnlock()
	result := nodeManager.setConsoleState(names, pbNodesStae.State)
	return &pb.Result{Result: result}, nil
}

func (cRPCServer *ConsoleRPCServer) serve() {
	var creds credentials.TransportCredentials
	var err error
	var s *grpc.Server
	if serverConfig.Global.SSLCACertFile != "" && serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" {
		tlsConfig, err := common.LoadServerTlsConfig(serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile, serverConfig.Global.SSLCACertFile)
		if err != nil {
			panic(err)
		}
		creds = credentials.NewTLS(tlsConfig)
	}
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", cRPCServer.host, cRPCServer.port))
	if err != nil {
		panic(err)
	}
	if creds != nil {
		s = grpc.NewServer(grpc.Creds(creds))
	} else {
		s = grpc.NewServer()
	}
	pb.RegisterConsoleManagerServer(s, cRPCServer)
	plog.Debug(fmt.Sprintf("Rpc server is listening on %s:%s", cRPCServer.host, cRPCServer.port))
	go s.Serve(lis)
}

type ConsoleRPCClient struct {
	host string
	port string
}

func newConsoleRPCClient(host string, port string) *ConsoleRPCClient {
	return &ConsoleRPCClient{host: host, port: port}
}

func (cRPCClient *ConsoleRPCClient) connect() (*grpc.ClientConn, error) {
	var creds credentials.TransportCredentials
	var err error
	var conn *grpc.ClientConn
	if serverConfig.Global.SSLCACertFile != "" && serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" {
		tlsConfig, err := common.LoadClientTlsConfig(serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile, serverConfig.Global.SSLCACertFile, cRPCClient.host)
		if err != nil {
			panic(err)
		}
		creds = credentials.NewTLS(tlsConfig)
	}
	addr := fmt.Sprintf("%s:%s", cRPCClient.host, cRPCClient.port)
	if creds != nil {
		conn, err = grpc.Dial(addr, grpc.WithTransportCredentials(creds))
	} else {
		conn, err = grpc.Dial(addr, grpc.WithInsecure())
	}
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	plog.Debug(fmt.Sprintf("Connect to %s to call the RPC method", addr))
	return conn, nil
}

func (cRPCClient *ConsoleRPCClient) ShowNode(name string) (*pb.Node, error) {
	conn, err := cRPCClient.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	c := pb.NewConsoleManagerClient(conn)
	node, err := c.ShowNode(context.Background(), &pb.NodeName{Name: name})
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	return node, nil
}

func (cRPCClient *ConsoleRPCClient) SetConsoleState(names []string, state string) (map[string]string, error) {
	conn, err := cRPCClient.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	c := pb.NewConsoleManagerClient(conn)
	pbResult, err := c.SetConsoleState(context.Background(), &pb.NodesState{Names: names, State: state})
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	return pbResult.Result, nil
}
