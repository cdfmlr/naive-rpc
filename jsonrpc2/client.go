package jsonrpc2

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
)

// TODO: client RPC 业务逻辑 和 传输层、编码层 分离

type Client interface {
	// Call a remote method with arg and return the result in ret.
	Call(method string, arg any, ret any) error
}

type client struct {
	serverAddr string
	nextId     atomic.Int64
}

func NewClient(serverAddr string) Client {
	return &client{
		serverAddr: serverAddr,
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

	// request -> json
	reqJson, err := json.Marshal(req)
	if err != nil {
		return err
	}

	// send request
	resp, err := http.Post(c.serverAddr, "application/json", bytes.NewReader(reqJson))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// parse response json
	var rpcResp Response
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return err
	}

	if rpcResp.Error != nil {
		return rpcResp.Error
	}

	// parse response result
	if ret == nil {
		return nil
	}

	if rpcResp.Result == nil {
		return errors.New("result should not be nil")
	}

	if err := json.Unmarshal(rpcResp.Result, ret); err != nil {
		return err
	}

	return nil
}
