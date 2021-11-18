package proxy

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var Routes = NewUpstream()
var hopHeaders = []string{
	"Connection",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"Te",
	"Trailers",
	"Transfer-Encoding",
	"Upgrade",
}

type Backend struct {
	/* Backend servers */
	Servers      []string
	MaxFail      int
	MaxPauseTime time.Duration

	/* Index of Servers, for load balance*/
	serverNumber int
	/* Record each host connect failed times */
	failTimes map[string]int
	/* Record bad backend and add back to Servers after MaxPauseTime */
	pause map[string]time.Time
}

/* Get load balance backend server */
func (b *Backend) GetBackendServer() *url.URL {
	if b.serverNumber >= len(b.Servers) {
		b.serverNumber = 0
	}
	server := b.Servers[b.serverNumber]
	uri, err := url.Parse(server)
	if err != nil {
		fmt.Println(err)
	}
	b.serverNumber++
	return uri
}

/* Modify Upstream list */
func (b *Backend) CheckUpstream(host string, statusCode int) {
	if b.failTimes == nil {
		b.failTimes = make(map[string]int)
	}
	if b.pause == nil {
		b.pause = make(map[string]time.Time)
	}
	if _, ok := b.failTimes[host]; !ok {
		b.failTimes[host] = 0
	}
	/* Set failTimes */
	if statusCode == http.StatusBadGateway {
		b.failTimes[host]++
	}
	/* Remove bad backend temporarily */
	if b.failTimes[host] >= b.MaxFail {
		var removeServer int
		for i, v := range b.Servers {
			if strings.Contains(v, host) {
				removeServer = i
				/* Record host pause time */
				b.pause[v] = time.Now()
				b.failTimes[host] = 0
				break
			}
		}
		var newServers = make([]string, len(b.Servers)-1)
		newServers = append(b.Servers[0:removeServer], b.Servers[removeServer+1:]...)
		b.Servers = newServers
	}
	/* Add backend after MaxPauseTime */
	if len(b.pause) != 0 {
		for k, v := range b.pause {
			if time.Since(v) > b.MaxPauseTime {
				b.Servers = append(b.Servers, k)
				delete(b.pause, k)
			}
		}
	}
}

/* Create default Backend */
func NewBackendDefault(servers []string) *Backend {
	return &Backend{
		Servers:      servers,
		MaxFail:      3,
		MaxPauseTime: 2 * time.Minute,
		failTimes:    make(map[string]int),
		pause:        make(map[string]time.Time),
	}
}

type Server struct {
	ServerName string
}

func NewServer(serverName string) *Server {
	return &Server{ServerName: serverName}
}

func BackendSelector(r *http.Request) (backend *Backend) {
	for k := range Routes.Sets {
		if ok := strings.Contains(k.ServerName, r.Host); ok {
			return Routes.Sets[k]
		}
	}
	return
}

type Upstream struct {
	Sets map[*Server]*Backend
}

func NewUpstream() *Upstream {
	return &Upstream{
		Sets: make(map[*Server]*Backend),
	}
}

func NewConfig(serverName string, backend *Backend) {
	s := NewServer(serverName)
	Routes.Sets[s] = backend
}

func DoRequest(r *http.Request, url *url.URL) *http.Response {
	/* Get raw method, body, and request uri */
	method := r.Method
	body := r.Body
	uri := url.Scheme + "://" + url.Host
	if path := r.URL.String(); path != "/" {
		uri = uri + path
	}
	if rawQuery := r.URL.RawQuery; rawQuery != "" {
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
	/* Modify headers */
	req.Header = r.Header
	req.RequestURI = ""
	addHeaders(req)
	deleteHeaders(req.Header)
	/* Do Request */
	client := http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			ExpectContinueTimeout: time.Second,
			DisableKeepAlives:     false,
			DisableCompression:    false,
			ForceAttemptHTTP2:     true,
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Request:    req,
		}
	}
	return resp
}

func ModifyResponse(rw http.ResponseWriter, r *http.Request) {
	backend := BackendSelector(r)
	server := backend.GetBackendServer()
	resp := DoRequest(r, server)
	backend.CheckUpstream(resp.Request.Host, resp.StatusCode)

	/* Parse body */
	var body []byte
	if resp.StatusCode == http.StatusBadGateway || resp.StatusCode == http.StatusGatewayTimeout {
		body = []byte(``)
	} else {
		bodyRaw, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			fmt.Println(err)
		}
		resp.Body.Close()
		body = bodyRaw
	}

	deleteHeaders(resp.Header)
	copyHeaders(resp.Header, rw.Header())
	rw.WriteHeader(resp.StatusCode)
	rw.Write(body)
}

func addHeaders(r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if origin, ok := r.Header["X-Forwarded-For"]; ok {
			ip = strings.Join(origin, ", ") + ", " + ip
		}
		r.Header.Set("X-Forwarded-For", ip)
	}
	r.Header.Set("X-Proxy-Enable", "true")
}

func deleteHeaders(header http.Header) {
	for _, v := range hopHeaders {
		header.Del(v)
	}
}

func copyHeaders(src, dst http.Header) {
	for k, v := range src {
		for _, vv := range v {
			dst.Add(k, vv)
		}
	}
}
