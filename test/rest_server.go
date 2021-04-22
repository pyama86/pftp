// This is test server for get domain name

package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
)

type response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type resource interface {
	URI() string
	Get(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response
	Post(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response
	Put(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response
	Delete(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response
}

type (
	postNotSupported   struct{}
	putNotSupported    struct{}
	deleteNotSupported struct{}
)

func (postNotSupported) Post(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response {
	return response{405, "", ""}
}

func (putNotSupported) Put(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response {
	return response{405, "", ""}
}

func (deleteNotSupported) Delete(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response {
	return response{405, "", ""}
}

func abort(rw http.ResponseWriter, statusCode int) {
	rw.WriteHeader(statusCode)
}

func httpResponse(rw http.ResponseWriter, req *http.Request, res response) {
	content, err := json.Marshal(res)

	if err != nil {
		abort(rw, 500)
	}

	rw.WriteHeader(res.Code)
	rw.Write(content)
}

func addResource(router *httprouter.Router, resource resource) {
	router.GET(resource.URI(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Get(rw, r, ps)
		httpResponse(rw, r, res)
	})
	router.POST(resource.URI(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Post(rw, r, ps)
		httpResponse(rw, r, res)
	})
	router.PUT(resource.URI(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Put(rw, r, ps)
		httpResponse(rw, r, res)
	})
	router.DELETE(resource.URI(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Delete(rw, r, ps)
		httpResponse(rw, r, res)
	})
}

// Testdomain data array
var domains = []string{
	"127.0.0.1:10021", // for vsuser
	"127.0.0.1:20021", // for prouser
}

type getUserDomain struct {
	postNotSupported
	putNotSupported
	deleteNotSupported
}

func (getUserDomain) URI() string {
	return "/getDomain"
}

func (getUserDomain) Get(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) response {
	user := r.FormValue("username")

	if user == "vsuser" {
		return response{200, "Username found", domains[0]}
	}

	if user == "prouser" {
		return response{200, "Username found", domains[1]}
	}

	return response{400, "Username not found", ""}
}

// LaunchTestRestServer Launch test server
func LaunchTestRestServer() (*http.Server, error) {
	router := httprouter.New()
	addResource(router, new(getUserDomain))

	srv := &http.Server{Addr: "127.0.0.1:8080", Handler: router}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			fmt.Println("unable run test webapi server")
		}
	}()

	return srv, nil
}

// LaunchUnitTestRestServer Launch test server for unit test
func LaunchUnitTestRestServer(t *testing.T) *httptest.Server {
	router := httprouter.New()
	addResource(router, new(getUserDomain))

	srv := httptest.NewServer(router)

	return srv
}
