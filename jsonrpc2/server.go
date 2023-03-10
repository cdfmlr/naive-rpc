package jsonrpc2

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"sync"
)

var Verbose = false

// RemoteProcess is a function that will be called by remote.
type RemoteProcess func(arg any) (ret any, err error)

// Server register methods and Serve JSON-RPC 2.0 over HTTP.
type Server interface {
	Register(name string, f any) error // register a method f with its name, while f is something like the RemoteProcess.
	ServeRPC(req *Request) *Response

	// WithAtMostOnce 是一个 Option: 执行 at-most-once 语意，消除重复 RPC 请求。
	//
	// WithAtMostOnce 原址设置当前 Server 执行 at-most-once，为了方便，该函数还会返回该 Server。
	//
	// e.g.
	//     s := NewServer().WithAtMostOnce()
	//     s.Register(...)
	//     st := NewHttpServerTransport(":6666")
	//     st.Serve(s)
	WithAtMostOnce() Server
}

// server is a Server implementation.
type server struct {
	mu      sync.RWMutex
	methods map[string]*method

	atMostOnce *sync.Map // nil: disable, else: 执行 at-most-once 语意，消除重复 RPC 请求
}

// NewServer creates JSON-RPC 2.0 Server.
func NewServer() Server {
	return &server{
		methods: make(map[string]*method),
	}
}

// WithAtMostOnce 原址设置当前 server 执行 at-most-once，并返回 Server 以供链式
func (s *server) WithAtMostOnce() Server {
	s.atMostOnce = new(sync.Map)
	return s
}

// Register registers a method f with its name.
func (s *server) Register(name string, f any) error {
	if _, exists := s.methods[name]; exists {
		return errors.New(fmt.Sprintf("multiple registrations for %s", name))
	}

	rp, err := newMethod(f)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.methods[name] = rp
	return nil
}

func (s *server) ServeRPC(req *Request) *Response {
	// find method
	s.mu.RLock()
	m, exists := s.methods[req.Method]
	s.mu.RUnlock()

	if !exists {
		return errorResponse(req.Id, ErrMethodNotFound())
	}

	if Verbose {
		log.Printf("ServeRPC request: method=%s, id=%d, params=%s\n", req.Method, *req.Id, req.Params)
	}

	if s.atMostOnce != nil && req.Id != nil {
		_, dup := s.atMostOnce.LoadOrStore(*req.Id, struct{}{})
		if dup {
			return errorResponse(req.Id, ErrAtMostOnce())
		}
	}

	// call method
	resp := m.serveRequest(req)

	if Verbose {
		log.Printf("ServeRPC response: id=%d, result=%s, error=%v\n", *resp.Id, resp.Result, resp.Error)
	}

	return resp
}

// method is the inner representation for a RemoteProcess.
type method struct {
	function reflect.Value
	inType   reflect.Type
	outType  reflect.Type
}

// newMethod constructs a method for given f.
// Errors if f invaild.
//
// newMethod = makeFunction + makeInType + makeOutType
func newMethod(f any) (*method, error) {
	rp := new(method)
	if err := rp.makeFunction(f); err != nil {
		return nil, err
	}
	if err := rp.makeInType(); err != nil {
		return nil, err
	}
	if err := rp.makeOutType(); err != nil {
		return nil, err
	}
	return rp, nil
}

// makeFunction fills the function field of the method.
//
// f should be something like a RemoteProcess.
func (p *method) makeFunction(f any) error {
	if f == nil {
		return errors.New("nil function")
	}

	fv := reflect.ValueOf(f)
	ft := fv.Type()

	if ft.Kind() != reflect.Func {
		return errors.New("not a Func")
	}

	p.function = fv
	return nil
}

// makeInType fills the inType field of the method.
// It should be called after makeFunction.
func (p *method) makeInType() error {
	ft := p.function.Type()

	if ft.NumIn() != 1 {
		return errors.New("exactly 1 parameter expected")
	}
	at := ft.In(0)

	p.inType = at
	return nil
}

// makeOutType fills the outType field of the method.
// It should be called after makeFunction.
func (p *method) makeOutType() error {
	ft := p.function.Type()

	if ft.NumOut() != 2 {
		return errors.New("exactly 2 return value (ret, err) expected")
	}

	errorInterface := reflect.TypeOf((*error)(nil)).Elem()
	if !ft.Out(1).Implements(errorInterface) {
		return errors.New("the 2nd return value should be an error")
	}

	p.outType = ft.Out(0)
	return nil
}

// deprecated: use request.unmarshalParam instead.
//
// unmarshalParam creates a param struct from given map.
// Returns the reflect.Value of a POINTER to the struct.
// This is intended to be passed to call().
//
// e.g. inType is Foo, returns reflect.ValueOf(Foo{})
func (p *method) unmarshalParam(params json.RawMessage) (reflect.Value, error) {
	req := Request{Params: params}
	return req.unmarshalParam(p.inType)
}

// call method with given param (reflect.ValueOf(Param{})) and returns the result (ret, err).
// Return values are NOT reflect.Value. They are the actual values (outType.Interface(), error).
// Panic will be recovered and returned as error.
func (p *method) call(param reflect.Value) (ret any, err error) {
	if param.Type() != p.inType {
		return nil, errors.New("param type mismatch")
	}

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered from method call: ", r)
			err = errors.New(fmt.Sprintf("panic: %v", r))
		}
	}()

	out := p.function.Call([]reflect.Value{param})

	if len(out) != 2 {
		return nil, errors.New("exactly 2 return value (ret, err) expected")
	}
	errorInterface := reflect.TypeOf((*error)(nil)).Elem()
	if !out[1].Type().Implements(errorInterface) {
		return nil, errors.New("the 2nd return value should be an error")
	}

	ret = out[0].Interface()
	e := out[1].Interface()
	if e != nil {
		return nil, e.(error)
	}
	return ret, nil
}

// serveRequest do unmarshalParam and call for a given request, returning the response.
func (p *method) serveRequest(req *Request) (res *Response) {
	if req == nil {
		return errorResponse(nil, ErrInvalidRequest().withReason("nil request"))
	}

	res = &Response{
		JsonRpc: JsonRpc2,
		Id:      req.Id,
	}

	// param, err := p.unmarshalParam(req.Params)  // deprecated
	param, err := req.unmarshalParam(p.inType)
	if err != nil {
		res.Error = ErrInvalidParams().withReason(err.Error())
		return
	}

	ret, err := p.call(param)
	if err != nil {
		res.Error = &Error{
			Code:    -1,
			Message: err.Error(),
		}
		return
	}

	if err = res.marshalResult(ret); err != nil {
		res.Result = nil
		res.Error = ErrInternalError().withReason(err.Error())
		return
	}

	return res
}
