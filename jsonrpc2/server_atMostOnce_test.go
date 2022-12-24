package jsonrpc2

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
)

func Test_server_AtMostOnce(t *testing.T) {
	s := NewServer().WithAtMostOnce()

	err := s.Register("add", func(arg *struct{ A, B int }) (*struct{ C int }, error) {
		return &struct{ C int }{C: arg.A + arg.B}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	chStart := make(chan struct{})
	chDoneTest := make(chan struct{})

	go func() {
		go func() {
			st := NewHttpServerTransport(":5677")
			close(chStart)
			err := st.Serve(s)
			if err != nil {
				t.Error(err)
				return
			}
		}()
		<-chDoneTest
	}()

	doRpcRequest := func(jsonBody string) *Response {
		resp, err := http.Post("http://localhost:5677/rpc-server-test", "application/json", bytes.NewBuffer([]byte(jsonBody)))
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
		{"good1",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 1, "B": 2}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Result: []byte(`{"C":3}`)}},
		{"dup1",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 2, "B": 3}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Error: ErrAtMostOnce()}},
		{"good2",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 1, "B": 2}, "id": 2}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(2), Result: []byte(`{"C":3}`)}},
		{"dup2",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 2, "B": 3}, "id": 2}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(2), Error: ErrAtMostOnce()}},
		{"dup1_again",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 2, "B": 3}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Error: ErrAtMostOnce()}},
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

func Test_server_NoAtMostOnce(t *testing.T) {
	s := NewServer()

	err := s.Register("add", func(arg *struct{ A, B int }) (*struct{ C int }, error) {
		return &struct{ C int }{C: arg.A + arg.B}, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	chStart := make(chan struct{})
	chDoneTest := make(chan struct{})

	go func() {
		go func() {
			st := NewHttpServerTransport(":5678")
			close(chStart)
			err := st.Serve(s)
			if err != nil {
				t.Error(err)
				return
			}
		}()
		<-chDoneTest
	}()

	doRpcRequest := func(jsonBody string) *Response {
		resp, err := http.Post("http://localhost:5678/rpc-server-test", "application/json", bytes.NewBuffer([]byte(jsonBody)))
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
		{"good1",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 1, "B": 2}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Result: []byte(`{"C":3}`)}},
		{"dup1",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 2, "B": 3}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Result: []byte(`{"C":5}`)}},
		{"good2",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 1, "B": 2}, "id": 2}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(2), Result: []byte(`{"C":3}`)}},
		{"dup2",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 2, "B": 3}, "id": 2}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(2), Result: []byte(`{"C":5}`)}},
		{"dup1_again",
			args{`{"jsonrpc": "2.0", "method": "add", "params": {"A": 2, "B": 3}, "id": 1}`},
			&Response{JsonRpc: JsonRpc2, Id: intPtr(1), Result: []byte(`{"C":5}`)}},
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
