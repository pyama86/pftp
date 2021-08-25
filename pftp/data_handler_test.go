package pftp

import (
	"reflect"
	"testing"

	"github.com/tevino/abool"
)

func Test_dataHandler_parsePORTcommand(t *testing.T) {
	type fields struct {
		line   string
		mode   string
		config *config
	}

	type want struct {
		ip   string
		port string
		err  string
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
				line:   "PORT 256,777,0,10,235,64\r\n",
				mode:   "PORT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "active_mode_invalid_port",
			fields: fields{
				line:   "PORT 10,10,10,10,530,64\r\n",
				mode:   "PORT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "active_mode_wrong_line",
			fields: fields{
				line:   "PORT (10,10,10,10,100,10(\r\n",
				mode:   "PORT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "active_mode_parse_ok",
			fields: fields{
				line:   "PORT 1,1,1,1,100,10\r\n",
				mode:   "PORT",
				config: &config{},
			},
			want: want{
				ip:   "1.1.1.1",
				port: "25610",
				err:  "",
			},
			wantErr: false,
		},
	}

	transferInTLS := abool.New()
	inDataTransfer := abool.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			d, _ := newDataHandler(
				tt.fields.config,
				nil,
				nil,
				nil,
				tt.fields.mode,
				nil,
				transferInTLS,
				inDataTransfer,
			)
			err := d.parsePORTcommand(tt.fields.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("dataHandler.parsePORTcommand() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}
			if err == nil {
				got.err = ""
			} else {
				got.err = err.Error()
			}

			got.ip = d.clientConn.remoteIP
			got.port = d.clientConn.remotePort
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.parsePORTresponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_dataHandler_parseEPRTcommand(t *testing.T) {
	type fields struct {
		line   string
		mode   string
		config *config
	}

	type want struct {
		ip   string
		port string
		err  string
	}

	tests := []struct {
		name    string
		fields  fields
		want    want
		wantErr bool
	}{
		{
			name: "eprt_mode_invalid_ip",
			fields: fields{
				line:   "EPRT |1|256.777.0.10|25610|\r\n",
				mode:   "EPRT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "eprt_mode_invalid_port",
			fields: fields{
				line:   "EPRT |1|10.10.10.10|73000|\r\n",
				mode:   "EPRT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "eprt_mode_invalid_protocol",
			fields: fields{
				line:   "EPRT |3|10.10.10.10|25610|\r\n",
				mode:   "EPRT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "unknown network protocol",
			},
			wantErr: true,
		},
		{
			name: "eprt_mode_wrong_line",
			fields: fields{
				line:   "EPRT |1|10.10.10.10|25610||\r\n",
				mode:   "EPRT",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "eprt_mode_parse_ok",
			fields: fields{
				line:   "EPRT |1|1.1.1.1|25610|\r\n",
				mode:   "EPRT",
				config: &config{},
			},
			want: want{
				ip:   "1.1.1.1",
				port: "25610",
				err:  "",
			},
			wantErr: false,
		},
	}

	transferInTLS := abool.New()
	inDataTransfer := abool.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			d, _ := newDataHandler(
				tt.fields.config,
				nil,
				nil,
				nil,
				tt.fields.mode,
				nil,
				transferInTLS,
				inDataTransfer,
			)
			err := d.parseEPRTcommand(tt.fields.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("dataHandler.parseEPRTcommand() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}
			if err == nil {
				got.err = ""
			} else {
				got.err = err.Error()
			}

			got.ip = d.clientConn.remoteIP
			got.port = d.clientConn.remotePort
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.parseEPRTresponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_dataHandler_parsePASVresponse(t *testing.T) {
	type fields struct {
		line   string
		mode   string
		config *config
	}

	type want struct {
		ip   string
		port string
		err  string
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
				line:   "227 Entering Passive Mode (256,777,0,10,235,64).\r\n",
				mode:   "PASV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "passive_mode_invalid_port",
			fields: fields{
				line:   "227 Entering Passive Mode (10,10,10,10,530,64).\r\n",
				mode:   "PASV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "passive_mode_wrong_line",
			fields: fields{
				line:   "227 Entering Passive Mode 10,10,10,10,100,10\r\n",
				mode:   "PASV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "passive_mode_parse_ok_public",
			fields: fields{
				line:   "227 Entering Passive Mode (20,30,40,50,100,10).\r\n",
				mode:   "PASV",
				config: &config{},
			},
			want: want{
				ip:   "20.30.40.50",
				port: "25610",
				err:  "",
			},
			wantErr: false,
		},
		{
			name: "passive_mode_parse_ok_private",
			fields: fields{
				line:   "227 Entering Passive Mode (10,30,40,50,100,10).\r\n",
				mode:   "PASV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "25610",
				err:  "",
			},
			wantErr: false,
		},
		{
			name: "passive_mode_parse_ignore_public_passive_ip",
			fields: fields{
				line: "227 Entering Passive Mode (20,30,40,50,100,10).\r\n",
				mode: "PASV",
				config: &config{
					IgnorePassiveIP: true,
				},
			},
			want: want{
				ip:   "",
				port: "25610",
				err:  "",
			},
			wantErr: false,
		},
	}

	transferInTLS := abool.New()
	inDataTransfer := abool.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			d, _ := newDataHandler(
				tt.fields.config,
				nil,
				nil,
				nil,
				tt.fields.mode,
				nil,
				transferInTLS,
				inDataTransfer,
			)
			err := d.parsePASVresponse(tt.fields.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("dataHandler.parsePASVresponse() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}
			if err == nil {
				got.err = ""
			} else {
				got.err = err.Error()
			}

			got.ip = d.originConn.remoteIP
			got.port = d.originConn.remotePort
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.parsePASVresponse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_dataHandler_parseEPSV(t *testing.T) {
	type fields struct {
		line   string
		mode   string
		config *config
	}

	type want struct {
		ip   string
		port string
		err  string
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
				line:   "229 Entering Extended Passive Mode (|||70000|)\r\n",
				mode:   "EPSV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "epsv_mode_wrong_line",
			fields: fields{
				line:   "229 Entering Extended Passive Mode (|||70000|\r\n",
				mode:   "EPSV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "",
				err:  "invalid data address",
			},
			wantErr: true,
		},
		{
			name: "epsve_mode_parse_ok",
			fields: fields{
				line:   "229 Entering Extended Passive Mode (|||25610|)\r\n",
				mode:   "EPSV",
				config: &config{},
			},
			want: want{
				ip:   "",
				port: "25610",
				err:  "",
			},
			wantErr: false,
		},
	}

	transferInTLS := abool.New()
	inDataTransfer := abool.New()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := want{}
			d, _ := newDataHandler(
				tt.fields.config,
				nil,
				nil,
				nil,
				tt.fields.mode,
				nil,
				transferInTLS,
				inDataTransfer,
			)
			err := d.parseEPSVresponse(tt.fields.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("dataHandler.parseEPSVresponse() error = %v, wantErr %v", got.err, tt.wantErr)
				return
			}
			if err == nil {
				got.err = ""
			} else {
				got.err = err.Error()
			}

			got.ip = d.originConn.remoteIP
			got.port = d.originConn.remotePort
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("dataHandler.parseEPSVresponse() = %v, want %v", got, tt.want)
			}
		})
	}
}
