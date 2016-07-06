package main

import (
	"flag"
	"net/http"
	"strconv"

	"github.com/google/trillian/ct_fe"
	"github.com/golang/glog"
)

var serverPortFlag = flag.Int("port", 8091, "Port to serve CT log requests on")

// TODO(Martin2112): Support TLS and other stuff, this is just to get started
func main() {
	flag.Parse()

	ct_fe.RegisterCTHandlers()
	glog.Warningf("Server exited: %v", http.ListenAndServe("localhost:" + strconv.Itoa(*serverPortFlag), nil))
}
