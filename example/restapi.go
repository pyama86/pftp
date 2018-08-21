package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/BurntSushi/toml"
)

type Config struct {
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
func GetDomainFromAPI(path string, param string) (*string, error) {
	var conf Config
	_, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return nil, err
	}

	req := fmt.Sprintf("%s:%s%s?username=%s", conf.URI, conf.PORT, conf.API, param)

	resp, err := http.Get(req)
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

	return &decodedBody.Data, nil
}
