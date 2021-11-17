package main

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {
	http.HandleFunc("/", reverse)
	err := http.ListenAndServe("0.0.0.0:80", nil)
	if err != nil {
		panic(err)
	}
}

type Backend struct {
	/* Backend servers */
	Upstreams    []string
	MaxFail      int
	MaxPauseTime time.Duration

	/* Index of Upstreams, for load balance*/
	serverNumber int
	/* Record each host connect failed times */
	failTimes map[string]int
	/* Record bad backend and add back to Upstreams after MaxPauseTime */
	pause map[string]time.Time
}

func (b *Backend) GetUrl() *url.URL {
	if b.serverNumber >= len(b.Upstreams) {
		b.serverNumber = 0
	}
	server := b.Upstreams[b.serverNumber]
	uri, err := url.Parse(server)
	if err != nil {
		fmt.Println(err)
	}
	b.serverNumber++
	return uri
}

func (b *Backend) CheckUpstream(host string, statusCode int) {
	if _, ok := b.failTimes[host]; !ok {
		b.failTimes[host] = 0
	}
	/* Set failTimes */
	if statusCode == http.StatusBadGateway {
		b.failTimes[host]++
	}
	/* Remove bad backend temporarily */
	if b.failTimes[host] >= b.MaxFail {
		var realServer int
		for i, v := range b.Upstreams {
			if strings.Contains(v, host) {
				realServer = i
				/* Record host pause time */
				b.pause[v] = time.Now()
				b.failTimes[host] = 0
				break
			}
		}
		var newUpstreams = make([]string, len(b.Upstreams)-1)
		newUpstreams = append(b.Upstreams[0:realServer], b.Upstreams[realServer+1:]...)
		b.Upstreams = newUpstreams
	}
	/* Add backend after MaxPauseTime */
	if len(b.pause) != 0 {
		for k, v := range b.pause {
			if time.Since(v) > b.MaxPauseTime {
				b.Upstreams = append(b.Upstreams, k)
				delete(b.pause, k)
			}
		}
	}
}

func reverse(rw http.ResponseWriter, r *http.Request) {
	rw.Header().Set("X-Proxy-Enable", "true")
	backend := Controller(r)
	url := backend.GetUrl()
	resp := Proxy(r, url)
	backend.CheckUpstream(resp.Request.Host, resp.StatusCode)

	/* Parse body */
	if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
		rw.WriteHeader(resp.StatusCode)
		rw.Write([]byte(nil))
	} else {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(err)
		}
		resp.Body.Close()
		rw.Write(body)
	}
}

func Proxy(r *http.Request, url *url.URL) *http.Response {
	/* Get raw method, body, and request uri */
	method := r.Method
	body := r.Body
	path := r.URL.Path
	rawQuery := r.URL.RawQuery
	host := url.Scheme + "://" + url.Host
	var uri string
	if path != "" {
		uri = host + path
	} else {
		uri = uri + host
	}
	if rawQuery != "" {
		uri = uri + "?" + rawQuery
	}
	/* Create Request */
	req, err := http.NewRequest(method, uri, body)
	if err != nil {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Request:    req,
		}
	}
	/* Do Request */
	client := http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Request:    req,
		}
	}
	return resp
}

func NewBackendDefault(servers []string) *Backend {
	return &Backend{
		Upstreams:    servers,
		MaxFail:      3,
		MaxPauseTime: 2 * time.Minute,
		failTimes:    make(map[string]int),
		pause:        make(map[string]time.Time),
	}
}

func Controller(r *http.Request) *Backend {
	var backend *Backend
	var github = NewBackendDefault([]string{"https://github.com"})
	var backendAP = NewBackendDefault([]string{"http://localhost:81", "http://localhost:82"})
	switch r.Header.Get("X-Test") {
	case "true":
		backend = backendAP
	default:
		backend = github
	}
	return backend
}
