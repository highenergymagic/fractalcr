package appliance

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"time"
)

func icmpChecksum(data []byte) uint16 {
	var sum uint32
	for len(data) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(data))
		data = data[2:]
	}
	if len(data) > 0 {
		sum += uint32(data[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}
func pingAddress(address string) (string, error) {
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return "", fmt.Errorf("Invalid IPv4 destination")
	}
	conn, err := net.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		return "", fmt.Errorf("Echo service is unavailable")
	}
	defer conn.Close()
	token := make([]byte, 10)
	if _, err = rand.Read(token); err != nil {
		return "", fmt.Errorf("Echo service request failed")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Sending 5, 100-byte ICMP Echos to %s, timeout is 2 seconds:\n", ip)
	received := 0
	minimum, maximum, total := time.Hour, time.Duration(0), time.Duration(0)
	for seq := 0; seq < 5; seq++ {
		packet := make([]byte, 100)
		packet[0] = 8
		copy(packet[4:6], token[:2])
		binary.BigEndian.PutUint16(packet[6:], uint16(seq))
		copy(packet[8:18], token)
		binary.BigEndian.PutUint16(packet[2:], icmpChecksum(packet))
		start := time.Now()
		_ = conn.SetDeadline(start.Add(2 * time.Second))
		if _, err = conn.WriteTo(packet, &net.IPAddr{IP: ip}); err != nil {
			b.WriteByte('.')
			continue
		}
		matched := false
		for {
			reply := make([]byte, 2048)
			n, from, e := conn.ReadFrom(reply)
			if e != nil {
				break
			}
			if n < 18 || from.String() != ip.String() || reply[0] != 0 || string(reply[4:8]) != string(packet[4:8]) || string(reply[8:18]) != string(token) {
				continue
			}
			elapsed := time.Since(start)
			if elapsed < minimum {
				minimum = elapsed
			}
			if elapsed > maximum {
				maximum = elapsed
			}
			total += elapsed
			received++
			matched = true
			break
		}
		if matched {
			b.WriteByte('!')
		} else {
			b.WriteByte('.')
		}
	}
	fmt.Fprintf(&b, "\nSuccess rate is %d percent (%d/5)", received*20, received)
	if received > 0 {
		fmt.Fprintf(&b, ", round-trip min/avg/max = %d/%d/%d ms", minimum.Milliseconds(), (total / time.Duration(received)).Milliseconds(), maximum.Milliseconds())
	}
	return b.String(), nil
}
