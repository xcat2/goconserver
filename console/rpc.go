package console

import (
	"context"
	"fmt"
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	"github.com/xcat2/goconserver/common"
	pb "github.com/xcat2/goconserver/console/consolepb"
	net_context "golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"net"
)

type ConsoleRpcServer struct {
	port   string
	host   string
	server *grpc.Server
}

func newConsoleRpcServer() *ConsoleRpcServer {
	return &ConsoleRpcServer{port: serverConfig.Etcd.RpcPort, host: serverConfig.Global.Host}
}

func (self *ConsoleRpcServer) ShowNode(ctx net_context.Context, rpcNode *pb.NodeName) (*pb.Node, error) {
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

func (self *ConsoleRpcServer) SetConsoleState(ctx net_context.Context, pbNodesStae *pb.NodesState) (*pb.Result, error) {
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

func (self *ConsoleRpcServer) GetReplayContent(ctx net_context.Context, rpcNode *pb.NodeName) (*pb.ReplayContent, error) {
	plog.Debug("Receive the RPC call GetReplayContent")
	if !nodeManager.Exists(rpcNode.Name) {
		plog.ErrorNode(rpcNode.Name, fmt.Sprintf("Not exist on %s", nodeManager.hostname))
		return nil, common.ErrNodeNotExist
	}
	// TODO: make ReplayLines more flexible
	content, err := nodeManager.pipeline.Fetch(rpcNode.Name, serverConfig.Console.ReplayLines)
	if err != nil {
		return nil, err
	}
	return &pb.ReplayContent{Content: content}, nil
}

func (self *ConsoleRpcServer) ListSessionUser(ctx net_context.Context, rpcNode *pb.NodeName) (pbUsers *pb.SessionUsers, err error) {
	plog.Debug("Receive the RPC call ListSessionUser")
	pbUsers = new(pb.SessionUsers)
	nodeManager.RWlock.RLock()
	if !nodeManager.Exists(rpcNode.Name) {
		nodeManager.RWlock.RUnlock()
		plog.ErrorNode(rpcNode.Name, fmt.Sprintf("Not exist on %s", nodeManager.hostname))
		return nil, common.ErrNodeNotExist
	}
	node := nodeManager.Nodes[rpcNode.Name]
	defer func() {
		if r := recover(); r != nil {
			pbUsers.Users = make([]string, 0)
			err = nil
		}
	}()
	users := node.console.ListSessionUser()
	nodeManager.RWlock.RUnlock()
	pbUsers.Users = users
	return pbUsers, nil
}

func (self *ConsoleRpcServer) ListNodesStatus(ctx net_context.Context, empty *google_protobuf.Empty) (*pb.NodesStatus, error) {
	plog.Debug("Receive the RPC call ListNodesStatus")
	nodesStatus := make(map[string]int32)
	nodeManager.RWlock.RLock()
	for name, node := range nodeManager.Nodes {
		nodesStatus[name] = int32(node.status)
	}
	nodeManager.RWlock.RUnlock()
	return &pb.NodesStatus{NodesStatus: nodesStatus}, nil
}

func (self *ConsoleRpcServer) serve() {
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
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%s", self.host, self.port))
	if err != nil {
		panic(err)
	}
	if creds != nil {
		s = grpc.NewServer(grpc.Creds(creds))
	} else {
		s = grpc.NewServer()
	}
	pb.RegisterConsoleManagerServer(s, self)
	plog.Debug(fmt.Sprintf("Rpc server is listening on %s:%s", self.host, self.port))
	go s.Serve(lis)
}

type ConsoleRpcClient struct {
	host string
	port string
}

func newConsoleRpcClient(host string, vhost string) (*ConsoleRpcClient, error) {
	config, err := nodeManager.stor.GetEndpoint(vhost)
	if err != nil {
		return nil, err
	}
	return &ConsoleRpcClient{host: host, port: config.RpcPort}, nil
}

func (self *ConsoleRpcClient) connect() (*grpc.ClientConn, error) {
	var creds credentials.TransportCredentials
	var err error
	var conn *grpc.ClientConn
	if serverConfig.Global.SSLCACertFile != "" && serverConfig.Global.SSLKeyFile != "" && serverConfig.Global.SSLCertFile != "" {
		tlsConfig, err := common.LoadClientTlsConfig(
			serverConfig.Global.SSLCertFile,
			serverConfig.Global.SSLKeyFile,
			serverConfig.Global.SSLCACertFile,
			self.host,
			false)
		if err != nil {
			panic(err)
		}
		creds = credentials.NewTLS(tlsConfig)
	}
	addr := fmt.Sprintf("%s:%s", self.host, self.port)
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

func (self *ConsoleRpcClient) ShowNode(name string) (*pb.Node, error) {
	conn, err := self.connect()
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

func (self *ConsoleRpcClient) SetConsoleState(names []string, state string) (map[string]string, error) {
	conn, err := self.connect()
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

func (self *ConsoleRpcClient) GetReplayContent(name string) (string, error) {
	conn, err := self.connect()
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

func (self *ConsoleRpcClient) ListSessionUser(name string) ([]string, error) {
	conn, err := self.connect()
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

func (self *ConsoleRpcClient) ListNodesStatus() (map[string]int, error) {
	conn, err := self.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	c := pb.NewConsoleManagerClient(conn)
	empty := new(google_protobuf.Empty)
	pbResult, err := c.ListNodesStatus(context.Background(), empty)
	if err != nil {
		plog.Error(err)
		return nil, err
	}
	ret := make(map[string]int)
	for k, v := range pbResult.NodesStatus {
		ret[k] = int(v)
	}
	return ret, nil
}
