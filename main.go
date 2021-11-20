package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

const (
	proxyAddr = "0.0.0.0:8000"
	server    = "http://127.0.0.1:8080/"
)

func getServerAddr() (*url.URL, error) {
	url, err := url.Parse(server)
	if err != nil {
		return nil, err
	}
	return url, nil
}

func proxy(w http.ResponseWriter, r *http.Request) {
	svrURL, err := getServerAddr()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	r.Host = svrURL.Host
	r.URL.Host = svrURL.Host
	r.URL.Scheme = svrURL.Scheme
	r.RequestURI = ""

	res, err := http.DefaultClient.Do(r)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}

	// Copy server response header to client response header
	for key, values := range res.Header {
		for _, v := range values {
			w.Header().Set(key, v)
		}
	}

	w.WriteHeader(res.StatusCode)
	io.Copy(w, res.Body)
}

func main() {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)

	r.Get("/api/*", proxy)

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "OK")
	})

	log.Println("Proxy server is running on port 8000")
	if err := http.ListenAndServe(proxyAddr, r); err != nil {
		log.Fatalln(err)
	}

}
