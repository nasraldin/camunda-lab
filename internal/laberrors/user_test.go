package laberrors

import (
	"fmt"
	"testing"
)

func TestWrapPortConflict(t *testing.T) {
	err := Wrap(fmt.Errorf("Bind for 0.0.0.0:8080 failed: port is already allocated"))
	u, ok := AsUser(err)
	if !ok || u.Code != "port_conflict" {
		t.Fatalf("got %#v ok=%v", u, ok)
	}
}

func TestContainerConflictRecoverable(t *testing.T) {
	u, ok := AsUser(ContainerConflict([]string{"postgres", "postgres"}))
	if !ok || !u.Recoverable || u.Code != "container_conflict" {
		t.Fatalf("got %#v", u)
	}
}
