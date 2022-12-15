package jsonrpc2

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sync"
)

// RemoteProcess is a function that will be called by remote.
type RemoteProcess func(arg any) (ret any, err error)

// Server register methods and Serve JSON-RPC 2.0 over HTTP.
// It's a http.Handler.
type Server interface {
	Register(name string, f any) error // register a method f with its name, while f is something like the RemoteProcess.
	http.Handler                       // ServeHTTP(ResponseWriter, *Request)
}

// server is a Server implementation.
type server struct {
	mu      sync.RWMutex
	methods map[string]*method
}

// NewServer creates JSON-RPC 2.0 Server.
func NewServer() Server {
	return &server{
		methods: make(map[string]*method),
	}
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

// ServeHTTP implements http.Handler. It serves JSON-RPC 2.0 over HTTP.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req Request

	// parse rpc request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		err := writeJsonResponse(w,
			ErrorResponse(nil, ErrParseError().WithReason(err.Error())))
		if err != nil {
			fmt.Println("Failed to write response: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	if err := req.validate(); err != nil {
		err := writeJsonResponse(w,
			ErrorResponse(req.Id, ErrInvalidRequest().WithReason(err.Error())))
		if err != nil {
			fmt.Println("Failed to write response: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	// find method
	s.mu.RLock()
	m, exists := s.methods[req.Method]
	s.mu.RUnlock()
	if !exists {
		err := writeJsonResponse(w,
			ErrorResponse(req.Id, ErrMethodNotFound()))
		if err != nil {
			fmt.Println("Failed to write response: ", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
		return
	}

	// call method
	resp := m.serveRequest(&req)

	// write response
	if err := writeJsonResponse(w, resp); err != nil {
		fmt.Println("Failed to write response: ", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeJsonResponse helps to respond with JSON content to the client.
func writeJsonResponse(w http.ResponseWriter, data any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(data)
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
	badValue := reflect.Zero(p.inType)
	dst := reflect.New(p.inType)

	if params == nil {
		return badValue, errors.New("params should not be nil")
	}

	// allow empty params: {} (not nil)
	//if len(params) == 0 {
	//	return reflect.Value{}, errors.New("params should not be empty")
	//}

	if err := json.Unmarshal(params, dst.Interface()); err != nil {
		return badValue, err
	}
	return dst.Elem(), nil
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
		return ErrorResponse(nil, ErrInvalidRequest().WithReason("nil request"))
	}

	res = &Response{
		JsonRpc: JsonRpc2,
		Id:      req.Id,
	}

	param, err := p.unmarshalParam(req.Params)
	if err != nil {
		res.Error = ErrInvalidParams().WithReason(err.Error())
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

	if err = res.setResult(ret); err != nil {
		res.Error = ErrInternalError().WithReason(err.Error())
		return
	}

	return res
}
