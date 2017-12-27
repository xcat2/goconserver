package console

import (
	"context"
	"fmt"
	"github.com/chenglch/goconserver/common"
	pb "github.com/chenglch/goconserver/console/consolepb"
	net_context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
	"path/filepath"
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
		plog.ErrorNode(rpcNode.Name, fmt.Sprintf("Not exist on %s", nodeManager.hostname))
		return nil, common.ErrNodeNotExist
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
			plog.ErrorNode(name, fmt.Sprintf("Could not find node on %s", nodeManager.hostname))
			continue
		}
		names = append(names, name)
	}
	nodeManager.RWlock.RUnlock()
	result := nodeManager.setConsoleState(names, pbNodesStae.State)
	return &pb.Result{Result: result}, nil
}

func (s *ConsoleRPCServer) GetReplayContent(ctx net_context.Context, rpcNode *pb.NodeName) (*pb.ReplayContent, error) {
	plog.Debug("Receive the RPC call GetReplayContent")
	if !nodeManager.Exists(rpcNode.Name) {
		plog.ErrorNode(rpcNode.Name, fmt.Sprintf("Not exist on %s", nodeManager.hostname))
		return nil, common.ErrNodeNotExist
	}
	logFile := fmt.Sprintf("%s%c%s.log", serverConfig.Console.LogDir, filepath.Separator, rpcNode.Name)
	content, err := common.ReadTail(logFile, serverConfig.Console.ReplayLines)
	if err != nil {
		plog.ErrorNode(rpcNode.Name, fmt.Sprintf("Could not read log file %s", logFile))
		return nil, err
	}
	return &pb.ReplayContent{Content: content}, nil
}

func (s *ConsoleRPCServer) ListSessionUser(ctx net_context.Context, rpcNode *pb.NodeName) (*pb.SessionUsers, error) {
	plog.Debug("Receive the RPC call ListSessionUser")
	nodeManager.RWlock.RLock()
	if !nodeManager.Exists(rpcNode.Name) {
		nodeManager.RWlock.RUnlock()
		plog.ErrorNode(rpcNode.Name, fmt.Sprintf("Not exist on %s", nodeManager.hostname))
		return nil, common.ErrNodeNotExist
	}
	node := nodeManager.Nodes[rpcNode.Name]
	users := node.console.ListSessionUser()
	nodeManager.RWlock.RUnlock()
	return &pb.SessionUsers{Users: users}, nil
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
		tlsConfig, err := common.LoadClientTlsConfig(
			serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile,
			serverConfig.Global.SSLCACertFile,
			cRPCClient.host,
			false)
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

func (cRPCClient *ConsoleRPCClient) GetReplayContent(name string) (string, error) {
	conn, err := cRPCClient.connect()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	c := pb.NewConsoleManagerClient(conn)
	pbResult, err := c.GetReplayContent(context.Background(), &pb.NodeName{Name: name})
	if err != nil {
		plog.Error(err)
		return "", err
	}
	return pbResult.Content, nil
}

func (cRPCClient *ConsoleRPCClient) ListSessionUser(name string) ([]string, error) {
	conn, err := cRPCClient.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	c := pb.NewConsoleManagerClient(conn)
	pbResult, err := c.ListSessionUser(context.Background(), &pb.NodeName{Name: name})
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	return pbResult.Users, nil
}
