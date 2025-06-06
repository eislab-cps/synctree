package helloworld

import (
	"errors"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type HelloWorld struct {
	msg string
}

func NewHelloWorld() *HelloWorld {
	err := errors.New("This is an error")

	if err != nil {
		log.WithFields(log.Fields{"Error": err}).Error("Error detected")
	}

	return &HelloWorld{
		msg: "Hello, World!",
	}
}

func (hello *HelloWorld) Talk() {
	log.WithFields(log.Fields{"Msg": hello.msg, "OtherMsg": "Logging is cool!"}).Info("Talking...")
	fmt.Println(hello.msg)
}
