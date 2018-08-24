// This is test server for get domain name

package test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/sirupsen/logrus"
)

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data"`
}

type Resource interface {
	Uri() string
	Get(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response
	Post(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response
	Put(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response
	Delete(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response
}

type (
	GetNotSupported    struct{}
	PostNotSupported   struct{}
	PutNotSupported    struct{}
	DeleteNotSupported struct{}
)

func (GetNotSupported) Get(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response {
	return Response{405, "", ""}
}

func (PostNotSupported) Post(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response {
	return Response{405, "", ""}
}

func (PutNotSupported) Put(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response {
	return Response{405, "", ""}
}

func (DeleteNotSupported) Delete(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response {
	return Response{405, "", ""}
}

func abort(rw http.ResponseWriter, statusCode int) {
	rw.WriteHeader(statusCode)
}

func HttpResponse(rw http.ResponseWriter, req *http.Request, res Response) {
	content, err := json.Marshal(res)

	if err != nil {
		abort(rw, 500)
	}

	rw.WriteHeader(res.Code)
	rw.Write(content)
}

func AddResource(router *httprouter.Router, resource Resource) {
	router.GET(resource.Uri(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Get(rw, r, ps)
		HttpResponse(rw, r, res)
	})
	router.POST(resource.Uri(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Post(rw, r, ps)
		HttpResponse(rw, r, res)
	})
	router.PUT(resource.Uri(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Put(rw, r, ps)
		HttpResponse(rw, r, res)
	})
	router.DELETE(resource.Uri(), func(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		res := resource.Delete(rw, r, ps)
		HttpResponse(rw, r, res)
	})
}

// Testdomain data array
var domains = []string{
	"127.0.0.1:10021", // for vsuser
	"127.0.0.1:21",    // for prouser
}

type GetUserDomain struct {
	PostNotSupported
	PutNotSupported
	DeleteNotSupported
}

func (GetUserDomain) Uri() string {
	return "/getDomain"
}

func (GetUserDomain) Get(rw http.ResponseWriter, r *http.Request, ps httprouter.Params) Response {
	user := r.FormValue("username")

	if user == "vsuser" {
		return Response{200, "Username found", domains[0]}
	}

	if user == "prouser" {
		return Response{200, "Username found", domains[1]}
	}

	return Response{400, "Username not found", ""}
}

func NewRestServer() (*http.Server, error) {
	router := httprouter.New()
	AddResource(router, new(GetUserDomain))

	srv := &http.Server{Addr: "127.0.0.1:8080", Handler: router}

	go func() {
		if err := srv.ListenAndServe(); err != nil {
			logrus.Fatal(err)
		}
	}()

	return srv, nil
}

func NewRestServer_Test(t *testing.T) *httptest.Server {
	router := httprouter.New()
	AddResource(router, new(GetUserDomain))

	srv := httptest.NewServer(router)

	return srv
}
