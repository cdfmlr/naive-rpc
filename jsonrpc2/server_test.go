package jsonrpc2

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func Test_newMethod(t *testing.T) {
	type argT struct{ A int }
	type retT struct{ B string }

	var (
		noArg       = func() (*retT, error) { return &retT{}, nil }
		tooManyArgs = func(a *argT, b int) (*retT, error) { return &retT{}, nil }
		retWrong    = func(a *argT) error { return nil }
		retNoErr    = func(a *argT) (int, float32) { return 1, 1.0 }
		expected    = func(a *argT) (*retT, error) { return &retT{}, nil }
		array       = func(a []int) (*retT, error) { return &retT{}, nil }
	)

	type args struct {
		f any
	}

	tests := []struct {
		name    string
		args    args
		want    *method
		wantErr bool
	}{
		{"nil", args{nil}, nil, true},
		{"int", args{1}, nil, true},
		{"emptyFunc", args{func() {}}, nil, true},
		{"noArg", args{noArg}, nil, true},
		{"tooManyArgs", args{tooManyArgs}, nil, true},
		{"retWrong", args{retWrong}, nil, true},
		{"retNoErr", args{retNoErr}, nil, true},
		{"expected", args{expected}, &method{
			function: reflect.ValueOf(expected),
			inType:   reflect.TypeOf(&argT{}),
			outType:  reflect.TypeOf(&retT{}),
		}, false},
		{"array", args{array}, &method{
			function: reflect.ValueOf(array),
			inType:   reflect.TypeOf([]int{}),
			outType:  reflect.TypeOf(&retT{}),
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newMethod(tt.args.f)
			if (err != nil) != tt.wantErr {
				t.Errorf("newMethod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newMethod() got = %v, want %v", got, tt.want)
			}
			t.Logf("✅ got=%v, err=%v", got, err)
		})
	}
}

func Test_method_unmarshalParam(t *testing.T) {
	type argT struct {
		A int
		B string
		C float32
		D bool
		E []int
		F map[string]int
		G struct{ H int }
	}

	var f = func(a argT) (struct{ B string }, error) { return struct{ B string }{}, nil }
	mObject, err := newMethod(f)
	if err != nil {
		t.Fatal(err)
	}

	var fP = func(a *argT) (*struct{ B string }, error) { return &struct{ B string }{}, nil }
	mPointer, err := newMethod(fP)
	if err != nil {
		t.Fatal(err)
	}

	nothing := []byte(``)
	emptyObject := []byte(`{}`)
	emptyArray := []byte(`[]`)
	str := []byte(`"str"`)
	num := []byte(`123`)
	goodObject := []byte(`{"a":1,"b":"2","c":3.0,"d":true,"e":[1,2,3],"f":{"1":1,"2":2,"3":3},"g":{"h":1}}`)
	badObject := []byte(`{"a":1.23,"b":2,"c":"3.0","d":true,"e":[1,2,3],"f":{"1":1,"2":2,"3":3},"g":{"h":1}}`)

	arrayF := func(a []int) (struct{ B string }, error) { return struct{ B string }{}, nil }
	mArray, err := newMethod(arrayF)
	if err != nil {
		t.Fatal(err)
	}

	goodArray := []byte(`[1,2,3]`)
	goodArrayT := []int{1, 2, 3}

	badArray := []byte(`[1,2,"3"]`)

	var argGood = argT{
		A: 1,
		B: "2",
		C: 3.0,
		D: true,
		E: []int{1, 2, 3},
		F: map[string]int{"1": 1, "2": 2, "3": 3},
		G: struct{ H int }{1},
	}

	type fields struct {
		function reflect.Value
		inType   reflect.Type
		outType  reflect.Type
	}
	type args struct {
		params json.RawMessage
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reflect.Value
		wantErr bool
	}{
		{"nil", fields(*mObject), args{params: nil}, reflect.ValueOf(argT{}), true},
		{"nothing", fields(*mObject), args{params: nothing}, reflect.ValueOf(argT{}), true},
		{"emptyObject", fields(*mObject), args{params: emptyObject}, reflect.ValueOf(argT{}), false},
		{"emptyArray", fields(*mObject), args{params: emptyArray}, reflect.ValueOf(argT{}), true},
		{"str", fields(*mObject), args{params: str}, reflect.ValueOf(argT{}), true},
		{"num", fields(*mObject), args{params: num}, reflect.ValueOf(argT{}), true},
		{"goodObject", fields(*mObject), args{params: goodObject}, reflect.ValueOf(argGood), false},
		{"badObject", fields(*mObject), args{params: badObject}, reflect.ValueOf(argT{}), true},
		{"goodPointer", fields(*mPointer), args{params: goodObject}, reflect.ValueOf(&argGood), false},
		{"badPointer", fields(*mPointer), args{params: badObject}, reflect.ValueOf((*argT)(nil)), true},
		{"goodArray", fields(*mArray), args{params: goodArray}, reflect.ValueOf(goodArrayT), false},
		{"badArray", fields(*mArray), args{params: badArray}, reflect.ValueOf([]int(nil)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &method{
				function: tt.fields.function,
				inType:   tt.fields.inType,
				outType:  tt.fields.outType,
			}
			got, err := p.unmarshalParam(tt.args.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalParam() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got.Interface(), tt.want.Interface()) {
				t.Errorf("unmarshalParam() got = %#v, want %#v", got, tt.want)
				return
			}
			t.Logf("✅ got = %#v, err = %v", got, err)
		})
	}
}

func Test_method_call(t *testing.T) {
	type argT struct {
		A int
		B int
	}
	arg := argT{A: 1, B: 2}

	type retT struct {
		C int
	}
	ret := retT{C: 3}

	fResult := func(a *argT) (*retT, error) {
		return &retT{C: a.A + a.B}, nil
	}
	mResult, err := newMethod(fResult)
	if err != nil {
		t.Fatal(err)
	}

	fError := func(a *argT) (*retT, error) {
		return nil, errors.New("error")
	}
	mError, err := newMethod(fError)
	if err != nil {
		t.Fatal(err)
	}

	fPanic := func(a *argT) (*retT, error) {
		panic("panic")
	}
	mPanic, err := newMethod(fPanic)
	if err != nil {
		t.Fatal(err)
	}

	type fields struct {
		function reflect.Value
		inType   reflect.Type
		outType  reflect.Type
	}
	type args struct {
		paramStruct reflect.Value
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    any
		wantErr bool
	}{
		{"result", fields(*mResult), args{paramStruct: reflect.ValueOf(&arg)}, &ret, false},
		{"error", fields(*mError), args{paramStruct: reflect.ValueOf(&arg)}, nil, true},
		{"badParam", fields(*mResult), args{paramStruct: reflect.ValueOf(1)}, nil, true},
		{"panic", fields(*mPanic), args{paramStruct: reflect.ValueOf(&arg)}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &method{
				function: tt.fields.function,
				inType:   tt.fields.inType,
				outType:  tt.fields.outType,
			}
			got, err := p.call(tt.args.paramStruct)
			if (err != nil) != tt.wantErr {
				t.Errorf("call() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("call() got = %v, want %v", got, tt.want)
			}
			t.Logf("✅ got = %v, err = %v", got, err)
		})
	}
}

func Test_method_serveRequest(t *testing.T) {
	intPtr := func(i int64) *int64 { return &i }

	f := func(a int) (int, error) {
		return a, nil
	}
	m, err := newMethod(f)
	if err != nil {
		t.Fatal(err)
	}

	type fields struct {
		function reflect.Value
		inType   reflect.Type
		outType  reflect.Type
	}
	type args struct {
		req *Request
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantRes *Response
	}{
		{"nil",
			fields(*m), args{req: nil},
			&Response{JsonRpc: JsonRpc2, Error: ErrInvalidRequest().WithReason("nil request")}},
		{"empty",
			fields(*m), args{req: &Request{}},
			&Response{JsonRpc: JsonRpc2, Id: nil, Error: ErrInvalidParams().WithReason("params should not be nil")}},
		{"noParam",
			fields(*m), args{req: &Request{Id: intPtr(1)}},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Error: ErrInvalidParams().WithReason("params should not be nil")}},
		{"good",
			fields(*m), args{req: &Request{Id: intPtr(1), Params: []byte(`2`)}},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Result: []byte(`2`)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &method{
				function: tt.fields.function,
				inType:   tt.fields.inType,
				outType:  tt.fields.outType,
			}
			gotRes := p.serveRequest(tt.args.req)
			if !reflect.DeepEqual(gotRes, tt.wantRes) {
				t.Errorf("serveRequest() = %#v, want %#v", gotRes, tt.wantRes)
			}
			jsonGot, _ := json.Marshal(gotRes)
			jsonWant, _ := json.Marshal(tt.wantRes)
			t.Logf("\ngot: %s\nwant:%s\n", jsonGot, jsonWant)
		})
	}
}

func Test_server_Register(t *testing.T) {
	s := NewServer()

	t.Run("nil", func(t *testing.T) {
		err := s.Register("add", nil)
		if err == nil {
			t.Fatal("expect error")
		}
		t.Log(err)
	})

	t.Run("noError", func(t *testing.T) {
		err := s.Register("add", func(a int) int { return a })
		if err == nil {
			t.Fatal("expect error")
		}
		t.Log(err)
	})

	t.Run("badParam", func(t *testing.T) {
		err := s.Register("add", func(a int, b int) (int, error) { return a + b, nil })
		if err == nil {
			t.Fatal(err)
		}
		t.Log(err)
	})

	t.Run("good", func(t *testing.T) {
		err := s.Register("add", func(arg *struct{ A, B int }) (*struct{ C int }, error) {
			return &struct{ C int }{C: arg.A + arg.B}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		t.Log(err)
	})

	t.Run("duplicate", func(t *testing.T) {
		err := s.Register("add", func(arg *struct{ A, B int }) (*struct{ C int }, error) {
			return &struct{ C int }{C: arg.A + arg.B}, nil
		})
		if err == nil {
			t.Fatal("expect error")
		}
		t.Log(err)
	})
}

func Test_server_ServeHTTP(t *testing.T) {
	s := NewServer()

	err := s.Register("add", func(arg *struct{ A, B int }) (*struct{ C int }, error) {
		return &struct{ C int }{C: arg.A + arg.B}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = s.Register("err", func(arg *struct{ A, B int }) (*struct{ C int }, error) {
		return nil, errors.New("error")
	})
	if err != nil {
		t.Fatal(err)
	}

	chStart := make(chan struct{})
	chDoneTest := make(chan struct{})

	go func() {
		go func() {
			http.Handle("/rpc-server-test", s)

			close(chStart)
			err := http.ListenAndServe(":5675", s)
			if err != nil {
				t.Error(err)
				return
			}
		}()
		<-chDoneTest
	}()

	doRpcRequest := func(jsonBody string) *Response {
		resp, err := http.Post("http://localhost:5675/rpc-server-test", "application/json", bytes.NewBuffer([]byte(jsonBody)))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}

		var res Response
		err = json.Unmarshal(body, &res)
		if err != nil {
			t.Fatal(err)
		}

		return &res
	}

	intPtr := func(i int64) *int64 {
		return &i
	}

	type args struct {
		json string
	}
	tests := []struct {
		name string
		args args
		want *Response
	}{
		{"good",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 1, "B": 2}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Result: []byte(`{"C":3}`)}},
		{"err",
			args{`{"jsonrpc": "2.0", "method": "err", "params": {"A": 1, "B": 2}, "id": 2}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(2), Error: &Error{Code: -1, Message: "error"}}},
		{"badMethod",
			args{`{"jsonrpc": "2.0", "method": "add1", "params": {"A": 1, "B": 2}, "id": 3}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(3), Error: ErrMethodNotFound()}},
		{"badParams",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": "foo"}, "id": 4}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(4), Error: ErrInvalidParams().WithReason("json: cannot unmarshal string into Go struct field .A of type int")}},
		{"badJson",
			args{`{"jsonrpc": "2.0", "met`},
			&Response{JsonRpc: JsonRpc2, Id: nil, Error: ErrParseError().WithReason("unexpected EOF")}},
	}

	<-chStart
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := doRpcRequest(tt.args.json)
			resJson, _ := json.Marshal(res)
			wantJson, _ := json.Marshal(tt.want)
			if !reflect.DeepEqual(resJson, wantJson) {
				t.Errorf("❌\ngot  = %s\nwant = %s\n", resJson, wantJson)
			} else {
				t.Logf("✅ got  = %s\n", resJson)
			}
		})
	}
	close(chDoneTest)
}
