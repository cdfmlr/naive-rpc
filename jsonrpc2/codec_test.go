package jsonrpc2

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRequest_unmarshalParam(t *testing.T) {
	type fields struct {
		Params json.RawMessage
	}
	type args struct {
		t reflect.Type
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    any
		wantErr bool
	}{
		{"nil", fields{nil}, args{nil}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Request{
				Params: tt.fields.Params,
			}
			got, err := r.unmarshalParam(tt.args.t)
			if (err != nil) != tt.wantErr {
				t.Errorf("unmarshalParam() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("unmarshalParam() got = %v, want %v", got, tt.want)
			}
		})
	}
}
