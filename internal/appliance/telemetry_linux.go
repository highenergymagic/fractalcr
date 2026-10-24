package appliance

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"fractalcr/internal/config"
	"golang.org/x/sys/unix"
	"math"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func storageStatus(c config.Config) (string, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/datastore/data0", &stat); err != nil || stat.Type != 0x9123683e {
		return "Datastore data0 is offline", nil
	}
	d := c.Datastores["data0"]
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bavail * uint64(stat.Bsize)
	return fmt.Sprintf("Datastore data0 is online\n  Filesystem: Btrfs\n  Data: %s\n  Disks: %s\n  %d bytes total (%d bytes free)\n", strings.ToUpper(d.Preset), strings.Join(d.Disks, ", "), total, free), nil
}
func resourceStatus() (string, error) {
	var info syscall.Sysinfo_t
	if err := syscall.Sysinfo(&info); err != nil {
		return "", fmt.Errorf("Platform resources are unavailable")
	}
	data, _ := os.ReadFile("/proc/meminfo")
	available := uint64(info.Freeram) * uint64(info.Unit)
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "MemAvailable:" {
			n, e := strconv.ParseUint(f[1], 10, 64)
			if e == nil {
				available = n * 1024
			}
		}
	}
	return fmt.Sprintf("Processor memory: %dK bytes total, %dK bytes available\nSwap capacity: %dK bytes total, %dK bytes free\nCPU load average: %.2f, %.2f, %.2f (1, 5, 15 minutes)\n", uint64(info.Totalram)*uint64(info.Unit)/1024, available/1024, uint64(info.Totalswap)*uint64(info.Unit)/1024, uint64(info.Freeswap)*uint64(info.Unit)/1024, float64(info.Loads[0])/65536, float64(info.Loads[1])/65536, float64(info.Loads[2])/65536), nil
}
func isolationStatus(c config.Config) string {
	state := "disabled"
	if c.Isolation.Enabled {
		state = "enabled"
	}
	accelerator := "available"
	if kvmAvailable() != nil {
		accelerator = "unavailable"
	}
	installed := "not installed"
	if _, err := os.Stat("/opt/kata/bin/containerd-shim-kata-v2"); err == nil {
		installed = "installed"
	}
	factory := "disabled"
	if c.Isolation.VMFactory {
		factory = "unavailable"
		if status, err := queryVMFactory("GET", "/status"); err == nil && status.Ready {
			factory = fmt.Sprintf("ready, %d MiB, created %s", status.MemoryMiB, status.Created.Format(time.RFC3339))
		}
	}
	backend := c.Engine.StorageBackend
	if backend == "" {
		backend = "overlay2"
	}
	return fmt.Sprintf("Container isolation is %s\n  Kata Containers: %s\n  Hardware virtualization: %s\n  Hypervisor: Cloud Hypervisor\n  Guest: %s\n  Default memory: %d MiB\n  Apply to: %s\n  Container storage: %s\n  VM factory: %s\n", state, installed, accelerator, c.Isolation.BaseOS, c.Isolation.MemoryMiB, c.Isolation.ApplyTo, backend, factory)
}

// Chrony's version-6 local monitoring protocol. No chronyc process or textual
// output parsing; packets are bounded and matched to command and sequence.
func chronyQuery(command, reply uint16, index uint32, size int) ([]byte, error) {
	var nonce [4]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("Time service request failed")
	}
	local := &net.UnixAddr{Name: fmt.Sprintf("/run/chrony/.fractal-%d-%x.sock", os.Getpid(), nonce), Net: "unixgram"}
	conn, err := net.DialUnix("unixgram", local, &net.UnixAddr{Name: "/run/chrony/chronyd.sock", Net: "unixgram"})
	if err != nil {
		return nil, fmt.Errorf("Time service is unavailable")
	}
	defer conn.Close()
	defer os.Remove(local.Name)
	// The directory is private to the time service; permit its dropped-privilege
	// worker to send the reply to this connected client socket.
	if err = os.Chmod(local.Name, 0622); err != nil {
		return nil, fmt.Errorf("Time service reply channel is unavailable")
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	packet := make([]byte, size)
	packet[0] = 6
	packet[1] = 1
	binary.BigEndian.PutUint16(packet[4:], command)
	copy(packet[8:], nonce[:])
	binary.BigEndian.PutUint32(packet[20:], index)
	if _, err = conn.Write(packet); err != nil {
		return nil, fmt.Errorf("Time service request failed")
	}
	response := make([]byte, 512)
	n, err := conn.Read(response)
	if err != nil {
		return nil, fmt.Errorf("Time service did not respond")
	}
	if n < size || response[0] != 6 || response[1] != 2 || binary.BigEndian.Uint16(response[4:]) != command || binary.BigEndian.Uint16(response[6:]) != reply || binary.BigEndian.Uint16(response[8:]) != 0 || string(response[16:20]) != string(nonce[:]) {
		return nil, fmt.Errorf("Invalid time service response")
	}
	return response[28:n], nil
}
func chronyFloat(b []byte) float64 {
	raw := binary.BigEndian.Uint32(b)
	exponent := int(int32(raw) >> 25)
	coefficient := int32(raw<<7) >> 7
	return math.Ldexp(float64(coefficient), exponent-25)
}
func chronyIP(b []byte) string {
	if binary.BigEndian.Uint16(b[16:]) == 1 {
		return net.IP(b[:4]).String()
	}
	if binary.BigEndian.Uint16(b[16:]) == 2 {
		return net.IP(b[:16]).String()
	}
	return "unspecified"
}

func clockStatus(now time.Time, detail bool) string {
	mark, source := "*", "none"
	var state syscall.Timex
	if result, err := syscall.Adjtimex(&state); err == nil && result != unix.TIME_ERROR && state.Status&unix.STA_UNSYNC == 0 {
		mark, source = "", "NTP"
	} else if _, err := os.Stat("/sys/class/rtc/rtc0"); err == nil && now.Year() >= 2025 {
		mark, source = ".", "hardware calendar"
	}
	output := mark + now.Format("15:04:05.000 MST Mon Jan 2 2006")
	if detail {
		output += "\nTime source is " + source
	}
	return output
}
func ntpStatus() (string, error) {
	data, err := chronyQuery(33, 5, 0, 104)
	if err != nil {
		return "", err
	}
	stratum := binary.BigEndian.Uint16(data[24:])
	state := "synchronized"
	if binary.BigEndian.Uint16(data[26:]) == 3 || stratum == 0 {
		state = "unsynchronized"
	}
	return fmt.Sprintf("Clock is %s, stratum %d, reference is %s\nClock offset is %.6f seconds\nRoot delay is %.6f seconds, root dispersion is %.6f seconds\n", state, stratum, chronyIP(data[4:24]), chronyFloat(data[44:48]), chronyFloat(data[64:68]), chronyFloat(data[68:72])), nil
}
func ntpAssociations() (string, error) {
	data, err := chronyQuery(14, 2, 0, 32)
	if err != nil {
		return "", err
	}
	count := binary.BigEndian.Uint32(data)
	if count > 64 {
		return "", fmt.Errorf("Time source count exceeds display limit")
	}
	var b strings.Builder
	b.WriteString("     address                                  st   poll   reach  offset(ms)\n")
	for i := uint32(0); i < count; i++ {
		d, e := chronyQuery(15, 3, i, 76)
		if e != nil {
			return "", e
		}
		mark := " "
		if binary.BigEndian.Uint16(d[24:]) == 0 {
			mark = "*"
		} else if binary.BigEndian.Uint16(d[24:]) == 5 {
			mark = "+"
		}
		poll := int16(binary.BigEndian.Uint16(d[20:]))
		fmt.Fprintf(&b, "%s    %-40s %2d %6.0f     %03o  %10.3f\n", mark, chronyIP(d[:20]), binary.BigEndian.Uint16(d[22:]), math.Pow(2, float64(poll)), binary.BigEndian.Uint16(d[30:]), chronyFloat(d[40:44])*1000)
	}
	b.WriteString("\n* sys.peer, + candidate\n")
	return b.String(), nil
}
