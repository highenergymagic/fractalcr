package appliance

import (
	"fmt"
	"os"
	"syscall"
)

func kvmAvailable() error {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("KVM is unavailable; expose hardware virtualization to enable Kata")
	}
	defer f.Close()
	version, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0xae00, 0)
	if errno != 0 || version != 12 {
		return fmt.Errorf("KVM API is unavailable")
	}
	vm, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0xae01, 0)
	if errno != 0 {
		return fmt.Errorf("KVM cannot create a VM: %v", errno)
	}
	syscall.Close(int(vm))
	return nil
}
