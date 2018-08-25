package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Apiserver ServerConfig `toml:"webapiserver"`
}

type ServerConfig struct {
	URL      string `toml:"url"`
	Port     string `toml:"port"`
	Endpoint string `toml:"endpoint"`
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
func RequestToServer(requestURL string, param string) (*Response, error) {
	requestURI := fmt.Sprintf("%s?username=%s", requestURL, param)

	resp, err := http.Get(requestURI)
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
	var conf Config
	_, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return nil, err
	}

	requestURL := fmt.Sprintf("%s:%s%s", conf.Apiserver.URL, conf.Apiserver.Port, conf.Apiserver.Endpoint)

	domain, err := RequestToServer(requestURL, param)
	if err != nil {
		return nil, err
	}

	return &domain.Data, nil
}
