package appliance

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

type Status struct {
	Name                           [32]byte
	State, PID, Enabled, Blocked   int32
	Starts, Failures, Dependencies uint32
}

var controlSocket = "/run/fractal-control.sock"

const supervisorProtocol = 2
const supervisorSlots = 32

func (s Status) Ready() bool   { return s.State == 2 && s.Blocked == 0 }
func (s Status) Label() string { return string(bytes.TrimRight(s.Name[:], "\x00")) }

func control(op uint32, name string) (map[string]Status, error) {
	c, err := net.DialTimeout("unixpacket", controlSocket, 2*time.Second)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	request := make([]byte, 40)
	binary.LittleEndian.PutUint32(request, supervisorProtocol)
	binary.LittleEndian.PutUint32(request[4:], op)
	copy(request[8:], name)
	if _, err = c.Write(request); err != nil {
		return nil, err
	}
	reply := make([]byte, 12+supervisorSlots*60)
	n, err := c.Read(reply)
	if err != nil {
		return nil, err
	}
	if n != len(reply) || binary.LittleEndian.Uint32(reply) != supervisorProtocol || binary.LittleEndian.Uint32(reply[4:]) > supervisorSlots {
		return nil, fmt.Errorf("invalid supervisor response")
	}
	if result := int32(binary.LittleEndian.Uint32(reply[8:])); result != 0 {
		return nil, fmt.Errorf("supervisor rejected request (%d)", result)
	}
	states := map[string]Status{}
	for i := 0; i < int(binary.LittleEndian.Uint32(reply[4:])); i++ {
		var s Status
		if err = binary.Read(bytes.NewReader(reply[12+i*60:12+(i+1)*60]), binary.LittleEndian, &s); err != nil {
			return nil, err
		}
		states[s.Label()] = s
	}
	return states, nil
}

func waitState(test func(map[string]Status) bool) error {
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		s, err := control(0, "")
		if err != nil {
			return err
		}
		if test(s) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("platform transition timed out; inspect show platform software service and show logging")
}
