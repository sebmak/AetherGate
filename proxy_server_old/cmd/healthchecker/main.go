package main

import (
	"net/http"
	"net/url"
	"os"

	goutil "github.com/sebmak/go_util"
)

func main() {
	args := os.Args[1:]
	if len(args) < 1 {
		goutil.Logger().Error("Not enough arguments: ./healthcheck URL")
		os.Exit(1)
	}
	u, err := url.Parse(args[0])

	if err != nil {
		goutil.Logger().Errorf("Error: %s", err.Error())
		os.Exit(1)
	}

	response, err := http.Get(u.String())

	if err != nil {
		goutil.Logger().Errorf("Error Connecting to %s: %s", u.String(), err.Error())
		os.Exit(1)
	} else if response.StatusCode != http.StatusOK {
		goutil.Logger().Infof("Status Check: %d", response.StatusCode)
		os.Exit(1)
	}
	goutil.Logger().Infof("Status Check: %d", response.StatusCode)
	os.Exit(0)
}
