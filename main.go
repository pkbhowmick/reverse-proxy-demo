package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
)

const (
	proxyAddr = "0.0.0.0:8000"
	MaxRetry  = 3
)

var (
	servers   []string        = []string{"http://127.0.0.1:8080", "http://127.0.0.1:8081"}
	isHealthy map[string]bool = make(map[string]bool)
	locker    sync.Mutex
)

func getServerAddr(svr string) (*url.URL, error) {
	url, err := url.Parse(svr)
	if err != nil {
		return nil, err
	}
	return url, nil
}

func proxy(w http.ResponseWriter, r *http.Request) {
	for try := 1; try <= MaxRetry; try++ {
		svr := rand.Intn(len(servers))
		svrURL, err := getServerAddr(servers[svr])
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

		if res.StatusCode >= 300 && try < MaxRetry {
			continue
		}

		// Copy server response header to client response header
		for key, values := range res.Header {
			for _, v := range values {
				w.Header().Set(key, v)
			}
		}

		w.WriteHeader(res.StatusCode)
		io.Copy(w, res.Body)
		return
	}
}

func healthChecker(ctx context.Context) {
	for {
		for _, s := range servers {
			healthPath := s + "/healthz"
			res, err := http.Get(healthPath)
			if err != nil {
				isHealthy[s] = false
				continue
			}
			if res.StatusCode == http.StatusOK {
				locker.Lock()
				isHealthy[s] = true
				locker.Unlock()
			} else {
				locker.Lock()
				isHealthy[s] = false
				locker.Unlock()
			}
		}
		time.Sleep(5 * time.Second)
	}
}

func getUnheathyServer(w http.ResponseWriter, r *http.Request) {
	locker.Lock()
	svrs := make([]string, 0)
	for s, ok := range isHealthy {
		if !ok {
			svrs = append(svrs, s)
		}
	}
	locker.Unlock()
	res := strings.Join(svrs, "\n")
	res = "Down servers:\n" + res
	w.Write([]byte(res))
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run healthChecker
	go healthChecker(ctx)

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.URLFormat)

	r.Get("/api/*", proxy)

	r.Group(func(r chi.Router) {
		// todo: add middleware to check admin authorization
		r.Route("/admin", func(r chi.Router) {
			r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, "OK")
			})
			r.Get("/list-unhealthy-servers", getUnheathyServer)
		})
	})

	log.Println("Proxy server is running on port 8000")
	if err := http.ListenAndServe(proxyAddr, r); err != nil {
		log.Fatalln(err)
	}

}
