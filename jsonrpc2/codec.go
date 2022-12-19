package jsonrpc2

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
)

// JsonRpc2 is the version of JSON-RPC 2.0.
const JsonRpc2 = "2.0"

// Request object for JSON-RPC 2.0
type Request struct {
	JsonRpc string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"` // delay parsing until we know the inType
	Id      *int64          `json:"id"`
}

// unmarshalRequest data into a Request object req.
func unmarshalRequest(data io.Reader, req *Request) error {
	return json.NewDecoder(data).Decode(req)
}

// unmarshalParam parses the Params into given type t.
// Returns the reflect.Value of a POINTER to the struct.
// This is intended to be passed to call().
//
// e.g. inType is Foo, returns reflect.ValueOf(Foo{})
func (r Request) unmarshalParam(inType reflect.Type) (reflect.Value, error) {
	if inType == nil {
		return reflect.Value{}, errors.New("inType should not be nil")
	}

	badValue := reflect.Zero(inType)
	dst := reflect.New(inType)

	if r.Params == nil {
		return badValue, errors.New("params should not be nil")
	}

	if err := json.Unmarshal(r.Params, dst.Interface()); err != nil {
		return badValue, err
	}
	return dst.Elem(), nil
}

func (r Request) validate() error {
	if r.JsonRpc != JsonRpc2 {
		return errors.New("invalid jsonrpc version")
	}
	if r.Method == "" {
		return errors.New("method should not be empty")
	}
	if r.Id == nil {
		return errors.New("id should not be nil")
	}
	return nil
}

// marshal r into w.
func (r Request) marshal(w io.Writer) error {
	return json.NewEncoder(w).Encode(r)
}

// toJSON marshals r into a byte slice.
func (r Request) toJSON() ([]byte, error) {
	return json.Marshal(r)
}

// Response object for JSON-RPC 2.0
type Response struct {
	JsonRpc string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	Id      *int64          `json:"id"` // int or null
}

// marshalResult fills the Result field with the given value.
func (r *Response) marshalResult(result any) error {
	if result == nil {
		return nil
	}

	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	r.Result = b
	return nil
}

// marshal marshals the response into a byte slice.
// This should be called after the Result or Error field is filled.
func (r *Response) marshal(w io.Writer) error {
	return json.NewEncoder(w).Encode(r)
}

// validate checks if the response is valid: either Result or Error is filled.
func (r *Response) validate() error {
	if r.JsonRpc != JsonRpc2 {
		return errors.New("invalid jsonrpc version")
	}
	// neither
	if r.Result == nil && r.Error == nil {
		return errors.New("either result or error should not be nil")
	}
	// both
	if r.Result != nil && r.Error != nil {
		return errors.New("either result or error should not be nil")
	}
	return nil
}

// unmarshalResponse data into a Response object resp.
func unmarshalResponse(data io.Reader, resp *Response) error {
	return json.NewDecoder(data).Decode(resp)
}

// unmarshalResult parses the Params into given type t.
// Return a pointer to the result value.
func (r *Response) unmarshalResult(dst any) error {
	if err := json.Unmarshal(r.Result, dst); err != nil {
		return err
	}
	return nil
}

// Error object for JSON-RPC 2.0
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error as a error.
func (e *Error) Error() string {
	s := fmt.Sprintf("jsonrpc2 error %d: %s", e.Code, e.Message)
	if e.Data != nil {
		s += fmt.Sprintf(" (%s)", e.Data)
	}
	return s
}

// withReason writes a detailed reason for the error in the Data field.
// The modifying is done in-place. Returning the error object itself is for chaining.
func (e *Error) withReason(reason string) *Error {
	data, _ := json.Marshal(map[string]string{"reason": reason})
	e.Data = data
	return e
}

// pre-defined errors
var (
	ErrParseError     = func() *Error { return &Error{Code: -32700, Message: "Parse error"} }      // Invalid JSON was received by the server. An error occurred on the server while parsing the JSON text.
	ErrInvalidRequest = func() *Error { return &Error{Code: -32600, Message: "Invalid Request"} }  // The JSON sent is not a valid Request object.
	ErrMethodNotFound = func() *Error { return &Error{Code: -32601, Message: "Method not found"} } // The function does not exist / is not available.
	ErrInvalidParams  = func() *Error { return &Error{Code: -32602, Message: "Invalid params"} }   // Invalid function parameter(s).
	ErrInternalError  = func() *Error { return &Error{Code: -32603, Message: "Internal error"} }   // Internal JSON-RPC error.
	ErrServerError    = func() *Error { return &Error{Code: -32000, Message: "Server error"} }     // -32000 to -32099: Reserved for implementation-defined server-errors.
)

// errorResponse helps to create a response for an error.
func errorResponse(id *int64, err *Error) *Response {
	return &Response{
		JsonRpc: JsonRpc2,
		Id:      id,
		Error:   err,
	}
}
