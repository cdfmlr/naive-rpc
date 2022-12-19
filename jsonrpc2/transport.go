package jsonrpc2

// 这个文件实现 RPC 的传输层。
//
// 注意 ServerTransport 和 ClientTransport 注入的方向不同，两个东西是对称的:
//
//     Server.ServeRPC->                       <-ClientTransport.SendAndReceive
// Server <------> ServerTransport <- net -> ClientTransport <------> Client
//     Request/Response                                   Request/Response
// [           codec              ]          [           codec              ]
//
// FIXME: 这个设计还有一点问题是，codec 与 server/client、transport 两头都是耦合的。
//        理想的情况应该是：
//  Server <- codec -> ServerTransport <- net -> ClientTransport <- codec -> Client

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
)

type ServerTransport interface {
	Serve(server Server) error
}

// HttpServerTransport serve jsonrpc2 over http.
// It's both a http.Handler and a ServerTransport.
type HttpServerTransport struct {
	ListenAddr string
	server     Server
}

func NewHttpServerTransport(listenAddr string) *HttpServerTransport {
	return &HttpServerTransport{ListenAddr: listenAddr}
}

// ServeHTTP implements http.Handler. It's used to serve jsonrpc2 over http.
// Must be called after Use to set the server else it will panic.
//
// Call ServeHTTP will ignore the listen address of HttpServerTransport.
func (t *HttpServerTransport) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if t.server == nil {
		panic("must call Use to set server before ServeHTTP")
	}

	var req Request

	// parse rpc request
	if err := unmarshalRequest(r.Body, &req); err != nil {
		err := writeJsonResponse(w,
			errorResponse(nil, ErrParseError().withReason(err.Error())))
		if err != nil {
			fmt.Println("Failed to write response: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	if err := req.validate(); err != nil {
		err := writeJsonResponse(w,
			errorResponse(req.Id, ErrInvalidRequest().withReason(err.Error())))
		if err != nil {
			fmt.Println("Failed to write response: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	resp := t.server.ServeRPC(&req)

	// write response
	if err := writeJsonResponse(w, resp); err != nil {
		fmt.Println("Failed to write response: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeJsonResponse helps to respond with JSON content to the client.
func writeJsonResponse(w http.ResponseWriter, response *Response) error {
	w.Header().Set("Content-Type", "application/json")
	if response == nil {
		return errors.New("nil response")
	}
	if err := response.validate(); err != nil {
		return err
	}
	return response.marshal(w)
}

// Use server to serve rpc requests.
func (t *HttpServerTransport) Use(server Server) {
	t.server = server
}

// Serve = Use + ServeHTTP
func (t *HttpServerTransport) Serve(server Server) error {
	t.Use(server)
	return http.ListenAndServe(t.ListenAddr, t)
}

type ClientTransport interface {
	SendAndReceive(req *Request) (*Response, error)
}

type HttpClientTransport struct {
	Addr string
}

func NewHttpClientTransport(addr string) *HttpClientTransport {
	return &HttpClientTransport{Addr: addr}
}

func (t *HttpClientTransport) SendAndReceive(req *Request) (*Response, error) {
	// request -> json
	reqJson, err := req.toJSON()
	if err != nil {
		return nil, err
	}

	// send request
	resp, err := http.Post(t.Addr, "application/json", bytes.NewReader(reqJson))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// parse response json
	var rpcResp Response
	if err := unmarshalResponse(resp.Body, &rpcResp); err != nil {
		return nil, err
	}

	return &rpcResp, nil
}
