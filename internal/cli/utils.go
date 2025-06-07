package cli

import (
	"github.com/eislab-cps/synctree/pkg/build"
	log "github.com/sirupsen/logrus"
	"os"
)

func CheckError(err error) {
	if err != nil {
		log.WithFields(log.Fields{"BuildVersion": build.BuildVersion, "BuildTime": build.BuildTime}).Error(err.Error())
		os.Exit(-1)
	}
}
