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
	Apiserver ServerConfig `toml:"apiserver"`
}

type ServerConfig struct {
	URI  string `toml:"uri"`
	PORT string `toml:"port"`
	API  string `toml:"api"`
}

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

// Request to server for get domain from username.
func RequestToServer(serverURL string, param string) (*Response, error) {
	requestURI := fmt.Sprintf("%s?username=%s", serverURL, param)

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

func GetDomainFromWebAPI(path string, param string) (*string, error) {
	var conf Config
	_, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return nil, err
	}

	serverURL := fmt.Sprintf("%s:%s%s", conf.Apiserver.URI, conf.Apiserver.PORT, conf.Apiserver.API)

	domain, err := RequestToServer(serverURL, param)
	if err != nil {
		return nil, err
	}

	return &domain.Data, nil
}
