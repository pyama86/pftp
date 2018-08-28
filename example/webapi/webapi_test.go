package webapi

import (
	"reflect"
	"testing"

	"github.com/pyama86/pftp/test"
)

func Test_restapi_RequestToServer(t *testing.T) {
	type fields struct {
		serverURI string
	}

	testsrv := test.LaunchUnitTestRestServer(t)
	defer testsrv.Close()

	tests := []struct {
		name    string
		fields  fields
		want    *Response
		wantErr bool
	}{
		{
			name: "vsuser",
			fields: fields{
				serverURI: testsrv.URL + "/getDomain?username=%s",
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
				serverURI: testsrv.URL + "/getDomain?username=%s",
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
				serverURI: testsrv.URL + "/getDomain?username=%s",
			},
			want: &Response{
				Code:    400,
				Message: "Username not found",
				Data:    "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := RequestToServer(tt.fields.serverURI, tt.name)
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
