package nativeguard

import (
	"errors"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"testing"
)

func TestExecGuard(t *testing.T) {
	if os.Getenv("FCR_GUARD_TEST_CHILD") != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestExecGuard$")
		command.Env = append(os.Environ(), "FCR_GUARD_TEST_CHILD=1")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("guard child: %v: %s", err, output)
		}
		return
	}
	if err := DenyExec(); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for i := 0; i < 16; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			err := exec.Command("/bin/true").Run()
			if !errors.Is(err, syscall.EPERM) {
				t.Errorf("execution was not denied: %v", err)
			}
		}()
	}
	workers.Wait()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}
