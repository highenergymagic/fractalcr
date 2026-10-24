package appliance

import (
	"encoding/binary"
	"fmt"
	"net"
	"time"
)

func storageRequest(operation uint32, profile string, disks []string) error {
	if len(disks) > 16 {
		return fmt.Errorf("too many datastore members")
	}
	message := make([]byte, 20+16*64)
	binary.LittleEndian.PutUint32(message, 0x46535431)
	binary.LittleEndian.PutUint32(message[4:], 1)
	binary.LittleEndian.PutUint32(message[8:], operation)
	if profile == "raid1" {
		binary.LittleEndian.PutUint32(message[12:], 1)
	} else if profile != "" && profile != "single" {
		return fmt.Errorf("invalid datastore profile")
	}
	binary.LittleEndian.PutUint32(message[16:], uint32(len(disks)))
	for i, disk := range disks {
		if len(disk) == 0 || len(disk) >= 64 {
			return fmt.Errorf("invalid disk identifier")
		}
		copy(message[20+i*64:], disk)
	}
	conn, err := net.DialTimeout("unixpacket", "/run/fractal-storage.sock", 5*time.Second)
	if err != nil {
		return fmt.Errorf("storage service is unavailable")
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Minute))
	if _, err = conn.Write(message); err != nil {
		return fmt.Errorf("storage service request failed")
	}
	reply := make([]byte, 13)
	n, err := conn.Read(reply)
	if err != nil || n != 12 || binary.LittleEndian.Uint32(reply) != 0x46535431 || binary.LittleEndian.Uint32(reply[4:]) != 1 {
		return fmt.Errorf("invalid storage service response")
	}
	switch binary.LittleEndian.Uint32(reply[8:]) {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("invalid storage operation or disk selection")
	case 2:
		return fmt.Errorf("an FRDATA datastore already exists; initialization would create an ambiguous label")
	case 3:
		return fmt.Errorf("cannot safely inspect existing filesystems")
	default:
		return fmt.Errorf("datastore initialization failed; inspect storage-format service logging")
	}
}
