package jsonrpc2

import (
	"encoding/json"
	"errors"
	"sync/atomic"
)

// TODO: client RPC 业务逻辑 和 传输层、编码层 分离

type Client interface {
	// Call a remote method with arg and return the result in ret.
	Call(method string, arg any, ret any) error
}

type client struct {
	transport ClientTransport
	nextId    atomic.Int64
}

func NewClient(transport ClientTransport) Client {
	return &client{
		transport: transport,
	}
}

func (c *client) Call(method string, arg any, ret any) error {
	// arg -> json
	if arg == nil {
		return errors.New("arg is nil")
	}

	argJson, err := json.Marshal(arg)
	if err != nil {
		return err
	}

	// build request

	id := c.nextId.Add(1)

	req := Request{
		JsonRpc: JsonRpc2,
		Method:  method,
		Params:  argJson,
		Id:      &id,
	}
	if err := req.validate(); err != nil {
		return err
	}

	// remote procedure call
	rpcResp, err := c.transport.SendAndReceive(&req)
	if err != nil {
		return err
	}

	// case 0: rpc error
	if rpcResp.Error != nil {
		return rpcResp.Error
	}

	// case 1: rpc success
	// parse response result
	if ret == nil {
		return nil
	}

	if rpcResp.Result == nil {
		return errors.New("result should not be nil")
	}

	if err := rpcResp.unmarshalResult(ret); err != nil {
		return err
	}

	return nil
}
