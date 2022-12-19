package jsonrpc2

// TODO: RPC 业务逻辑（server、client） 和 传输层 分离

type Transport interface {
	ListenAndServe(addr string, server Server) error
}
