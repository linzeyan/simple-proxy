package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	proxy "github.com/linzeyan/simple-proxy"
	"github.com/spf13/viper"
)

const (
	usage = `Simple Proxy

Usage: proxy [option]

Options:
`
)

var (
	port   = flag.String("p", "80", "Specify listen port")
	config = flag.String("f", "proxy.yaml", "Specify config path")
)

func main() {
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	conf := readConfig()
	for k := range conf {
		proxy.NewConfig(
			viper.GetString(k+".serverName"),
			proxy.NewBackendDefault(viper.GetStringSlice(k+".upstream")),
		)
	}

	http.HandleFunc("/", proxy.ModifyResponse)
	err := http.ListenAndServe("0.0.0.0:"+*port, nil)
	if err != nil {
		panic(err)
	}
}

func readConfig() map[string]interface{} {
	if *config != "" {
		viper.SetConfigType("yaml")
		viper.SetConfigFile(*config)
	} else {
		viper.SetConfigType("yaml")
		viper.AddConfigPath("$HOME")
		viper.SetConfigName("proxy")
	}
	viper.AutomaticEnv()
	if err := viper.ReadInConfig(); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
	return viper.AllSettings()
}
