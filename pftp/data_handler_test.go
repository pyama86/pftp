package pftp

import (
	"fmt"
	"reflect"
	"testing"
)

func Test_dataHandler_parsePORT(t *testing.T) {
	type fields struct {
		line string
		mode string
	}

	type want struct {
		ip   string
		port int
		err  error
	}

	tests := []struct {
		name    string
		fields  fields
		want    want
		wantErr bool
	}{
		{
			name: "active_mode_invalid_ip",
			fields: fields{
				line: "PORT 256,777,0,10,235,64\r\n",
				mode: "PORT",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "active_mode_invalid_port",
			fields: fields{
				line: "PORT 10,10,10,10,530,64\r\n",
				mode: "PORT",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "active_mode_wrong_line",
			fields: fields{
				line: "PORT (10,10,10,10,100,10(\r\n",
				mode: "PORT",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "active_mode_parse_ok",
			fields: fields{
				line: "PORT 10,10,10,10,100,10\r\n",
				mode: "PORT",
			},
			want: want{
				ip:   "10.10.10.10",
				port: 25610,
				err:  nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			got.ip, got.port, got.err = newDataHandler(
				tt.fields.line,
				"",
				nil,
				nil,
				tt.fields.mode)
			if (got.err != nil) != tt.wantErr {
				t.Errorf("dataHandler.newDataListener() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}

			if tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.newDataListener() = %s, want %s", got.err.Error(), tt.want.err.Error())
			}
		})
	}
}

func Test_dataHandler_parsePASV(t *testing.T) {
	type fields struct {
		line string
		mode string
	}

	type want struct {
		ip   string
		port int
		err  error
	}

	tests := []struct {
		name    string
		fields  fields
		want    want
		wantErr bool
	}{
		{
			name: "passive_mode_invalid_ip",
			fields: fields{
				line: "227 Entering Passive Mode (256,777,0,10,235,64).\r\n",
				mode: "PASV",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "passive_mode_invalid_port",
			fields: fields{
				line: "227 Entering Passive Mode (10,10,10,10,530,64).\r\n",
				mode: "PASV",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "passive_mode_wrong_line",
			fields: fields{
				line: "227 Entering Passive Mode 10,10,10,10,100,10\r\n",
				mode: "PASV",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "passive_mode_parse_ok",
			fields: fields{
				line: "227 Entering Passive Mode (10,19,10,10,100,10).\r\n",
				mode: "PASV",
			},
			want: want{
				ip:   "10.10.10.10",
				port: 25610,
				err:  nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			got.ip, got.port, got.err = newDataHandler(
				tt.fields.line,
				"",
				nil,
				nil,
				tt.fields.mode)
			if (got.err != nil) != tt.wantErr {
				t.Errorf("dataHandler.newDataListener() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}

			if tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.newDataListener() = %s, want %s", got.err.Error(), tt.want.err.Error())
			}
		})
	}
}

func Test_dataHandler_parseEPSV(t *testing.T) {
	type fields struct {
		line string
		mode string
	}

	type want struct {
		ip   string
		port int
		err  error
	}

	tests := []struct {
		name    string
		fields  fields
		want    want
		wantErr bool
	}{
		{
			name: "epsv_mode_invalid_port",
			fields: fields{
				line: "229 Entering Extended Passive Mode (|||70000|)\r\n",
				mode: "EPSV",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "epsv_mode_wrong_line",
			fields: fields{
				line: "229 Entering Extended Passive Mode (|||70000|\r\n",
				mode: "EPSV",
			},
			want: want{
				ip:   "",
				port: -1,
				err:  fmt.Errorf("invalid data address"),
			},
			wantErr: true,
		},
		{
			name: "epsve_mode_parse_ok",
			fields: fields{
				line: "229 Entering Extended Passive Mode (|||25610|)\r\n",
				mode: "EPSV",
			},
			want: want{
				ip:   "",
				port: 25610,
				err:  nil,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			got.ip, got.port, got.err = newDataHandler(
				tt.fields.line,
				"",
				nil,
				nil,
				tt.fields.mode)
			if (got.err != nil) != tt.wantErr {
				t.Errorf("dataHandler.newDataListener() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}

			if tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.newDataListener() = %s, want %s", got.err.Error(), tt.want.err.Error())
			}
		})
	}
}
