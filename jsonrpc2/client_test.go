package jsonrpc2

import (
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func Test_client_Call(t *testing.T) {
	type StubArg struct {
		A int
		B int
	}
	type StubRet struct {
		C int
	}

	chServerStart := make(chan struct{})
	chDoneTest := make(chan struct{})

	// server
	go func() {
		s := NewServer()

		err := s.Register("add", func(arg *StubArg) (*StubRet, error) {
			return &StubRet{C: arg.A + arg.B}, nil
		})
		if err != nil {
			t.Error(err)
			return
		}

		err = s.Register("err", func(arg *StubArg) (*StubRet, error) {
			return nil, errors.New("error")
		})
		if err != nil {
			t.Error(err)
			return
		}

		go func() {
			http.Handle("/rpc-client-test", s)
			close(chServerStart)
			err := http.ListenAndServe(":5676", s)
			if err != nil {
				t.Error(err)
				return
			}
		}()

		<-chDoneTest
	}()

	// client

	cli := NewClient("http://localhost:5676/rpc-client-test")

	//intPtr := func(i int64) *int64 {
	//	return &i
	//}

	type args struct {
		method string
		arg    any
	}
	tests := []struct {
		name    string
		cli     Client
		args    args
		want    any
		wantErr bool
	}{
		{"add", cli, args{"add", &StubArg{A: 1, B: 2}}, &StubRet{C: 3}, false},
		{"err", cli, args{"err", &StubArg{A: 1, B: 2}}, new(StubRet), true},
		{"badMethod", cli, args{"badMethod", &StubArg{A: 1, B: 2}}, new(StubRet), true},
		{"badArg_nil", cli, args{"add", nil}, new(StubRet), true},
		{"badArg_other", cli, args{"add", []int{6, 6}}, new(StubRet), true},
	}

	<-chServerStart
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := new(StubRet)
			err := tt.cli.Call(tt.args.method, tt.args.arg, got)
			if (err != nil) != tt.wantErr {
				t.Errorf("client.Call() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("❌\ngot  = %v\nwant = %v\n", got, tt.want)
			} else {
				t.Logf("✅ got  = %v, err = %v\n", got, err)
			}
		})
	}
	close(chDoneTest)
}
