package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/BurntSushi/toml"
)

type config struct {
	Apiserver serverConfig `toml:"webapiserver"`
}

type serverConfig struct {
	URI string `toml:"uri"`
}

// Response from server will contain 3 elements with JSON type.
// {
//	  code : http response code
//	  message : response message from server
//	  data : destination url
// }
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// RequestToServer will return response data from webapi server
// If response code doesn't got 2xx, return error.
func RequestToServer(requestURI string, param string) (*Response, error) {
	resp, err := http.Get(fmt.Sprintf(requestURI, param))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var decodedBody = new(Response)
	json.Unmarshal(respBody, &decodedBody)

	if decodedBody.Code != 200 {
		return nil, errors.New(decodedBody.Message)
	}

	return decodedBody, nil
}

// GetDomainFromWebAPI will return destination url by string.
// Make request URL from config file and has request to server with username parameter.
func GetDomainFromWebAPI(path string, param string) (*string, error) {
	var conf config
	_, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return nil, err
	}

	domain, err := RequestToServer(conf.Apiserver.URI, param)
	if err != nil {
		return nil, err
	}

	return &domain.Data, nil
}
