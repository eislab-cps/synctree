package main

import (
	"github.com/eislab-cps/synctree/internal/cli"
	"github.com/eislab-cps/synctree/pkg/build"
)

var (
	BuildVersion string = ""
	BuildTime    string = ""
)

func main() {
	build.BuildVersion = BuildVersion
	build.BuildTime = BuildTime
	cli.Execute()
}
