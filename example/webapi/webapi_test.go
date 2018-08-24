package webapi

import (
	"reflect"
	"testing"

	"github.com/pyama86/pftp/test"
)

func Test_restapi_RequestToServer(t *testing.T) {
	serverready := make(chan struct{})

	type fields struct {
		config string
	}

	go test.NewRestServer_Test(serverready, t)

	tests := []struct {
		name    string
		fields  fields
		want    *Response
		wantErr bool
	}{
		{
			name: "vsuser",
			fields: fields{
				config: "http://127.0.0.1:8080/getDomain",
			},
			want: &Response{
				Code:    200,
				Message: "Username found",
				Data:    "127.0.0.1:10021",
			},
			wantErr: false,
		},
		{
			name: "prouser",
			fields: fields{
				config: "http://127.0.0.1:8080/getDomain",
			},
			want: &Response{
				Code:    200,
				Message: "Username found",
				Data:    "127.0.0.1:21",
			},
			wantErr: false,
		},
		{
			name: "hogemoge",
			fields: fields{
				config: "http://127.0.0.1:8080/getDomain",
			},
			want: &Response{
				Code:    400,
				Message: "Username not found",
				Data:    "",
			},
			wantErr: true,
		},
	}

	<-serverready
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequestToServer(tt.fields.config, tt.name)
			if err != nil {
				if !tt.wantErr || (err.Error() != tt.want.Message) {
					t.Errorf("got error when request to server = %v", err)
				}
			}

			if !reflect.DeepEqual(got, tt.want) && !tt.wantErr {
				t.Errorf("restapi.RequestToServer() = %v, want %v", got, tt.want)
			}
		})
	}
}
