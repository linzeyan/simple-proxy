package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	proxy "github.com/linzeyan/simple-proxy"
)

const (
	usage = `Simple Proxy

Usage: proxy [option]

Options:
`
)

var port = flag.String("p", "80", "Specify listen port")

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()
	var github = proxy.NewBackendDefault([]string{"https://github.com"})
	var backendAP = proxy.NewBackendDefault([]string{"http://localhost:81", "http://localhost:82"})
	proxy.NewRoutes("http://git.com", github)
	proxy.NewRoutes("http://test.com", backendAP)
	http.HandleFunc("/", proxy.ModifyResponse)
	err := http.ListenAndServe("0.0.0.0:"+*port, nil)
	if err != nil {
		panic(err)
	}
}
