package helloworld

import (
	"testing"
)

func TestNewHelloWorld(t *testing.T) {
	hw := NewHelloWorld()

	expectedMessage := "Hello, World!"
	if hw.msg != expectedMessage {
		t.Errorf("expected msg to be %q, but got %q", expectedMessage, hw.msg)
	}
}
