package appliance

import (
	"context"
	"encoding/json"
	"fmt"
	"fractalcr/internal/config"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func notifyReady() {
	if fd, e := strconv.Atoi(os.Getenv("FCR_READY_FD")); e == nil {
		f := os.NewFile(uintptr(fd), "ready")
		f.Write([]byte{'R'})
		f.Close()
		os.Unsetenv("FCR_READY_FD")
	}
}
func leasePath(n string) string { return "/run/network/" + n + ".json" }
func removeAddress(n config.Network) {
	_ = staticRoutes(n, false, true)
	if n.Gateway != "" {
		_ = gatewayRequest(n.Interface, n.Gateway, true)
	}
	if n.Address != "" {
		_ = addressRequest(n.Interface, n.Address, true)
	}
}
func clearLease(name string) {
	b, e := os.ReadFile(leasePath(name))
	if e != nil {
		return
	}
	var old config.Network
	if json.Unmarshal(b, &old) == nil && old.Interface == name {
		removeAddress(old)
		if old.IsManagement() {
			_ = config.AtomicWrite("/run/resolv.conf", nil, 0644)
		}
	}
	os.Remove(leasePath(name))
}
func applyAddress(n config.Network) error {
	if n.Address != "" {
		if e := addressRequest(n.Interface, n.Address, false); e != nil {
			return e
		}
	}
	if n.Gateway != "" {
		if e := gatewayRequest(n.Interface, n.Gateway, false); e != nil {
			return e
		}
	}
	if n.IsManagement() {
		dns := ""
		if n.DNS != "" {
			dns = "nameserver " + n.DNS + "\n"
		}
		if err := config.AtomicWrite("/run/resolv.conf", []byte(dns), 0644); err != nil {
			return err
		}
	}
	return staticRoutes(n, false, false)
}

// NetworkService owns only its configured addresses and DHCP clients. A DHCP
// server outage does not suppress the physical console or unrelated NICs.
func NetworkService() error {
	c, e := config.Load("/run/startup-config.json")
	if e != nil {
		return e
	}
	migrateInterfaces(&c)
	if e = os.MkdirAll("/run/network", 0700); e != nil {
		return e
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	var workers sync.WaitGroup
	stopped := make(chan error, len(c.AllInterfaces()))
	defer func() {
		cancel()
		workers.Wait()
		for name, n := range c.AllInterfaces() {
			_ = interfaceAddresses(*n, true)
			if n.DHCP {
				clearLease(name)
			} else {
				removeAddress(*n)
			}
		}
		removeVirtualInterfaces(c)
	}()
	if e = config.AtomicWrite("/run/resolv.conf", nil, 0644); e != nil {
		return e
	}
	for _, name := range c.InterfaceOrder() {
		if _, kind, _, _ := config.VirtualInterface(name); kind == "" {
			if e = setInterfaceMTU(name, c.InterfaceMTU(name)); e != nil {
				return e
			}
		}
	}
	if e = ensureVirtualInterfaces(c); e != nil {
		return e
	}
	for _, name := range c.InterfaceOrder() {
		n := c.AllInterfaces()[name]
		if _, kind, _, _ := config.VirtualInterface(name); kind == "vlan" && n.Parent == "" {
			continue
		}
		if e = setInterfaceMTU(name, c.InterfaceMTU(name)); e != nil {
			return e
		}
		if e = setInterfaceState(name, !n.Shutdown); e != nil {
			return e
		}
		if n.Shutdown {
			continue
		}
		if e = interfaceAddresses(*n, false); e != nil {
			return e
		}
		if !n.DHCP {
			if e = applyAddress(*n); e != nil {
				return e
			}
			continue
		}
		clearLease(name)
		kernel, err := kernelInterface(name)
		if err != nil {
			return err
		}
		workers.Add(1)
		go func(base config.Network, device string) { defer workers.Done(); stopped <- runDHCP(ctx, base, device) }(*n, kernel)
	}
	notifyReady()
	select {
	case <-ctx.Done():
		return nil
	case e := <-stopped:
		return fmt.Errorf("DHCP client exited: %v", e)
	}
}

func TimeService() error {
	c, e := config.Load("/run/startup-config.json")
	if e != nil {
		return e
	}
	account, e := user.Lookup("chronyd")
	if e != nil {
		return e
	}
	uid, e := strconv.Atoi(account.Uid)
	if e != nil {
		return e
	}
	gid, e := strconv.Atoi(account.Gid)
	if e != nil {
		return e
	}
	for _, dir := range []string{"/run/chrony", "/datastore/data0/chrony"} {
		if e = os.MkdirAll(dir, 0750); e != nil {
			return e
		}
		if e = os.Chown(dir, uid, gid); e != nil {
			return e
		}
	}
	// Older datastores have a private root directory. Give the unprivileged
	// daemon a dedicated view of its state, without traversing that directory.
	// The mountpoint's parent is root-owned; chronyd cannot replace it.
	const state = "/run/fractal-time-state"
	_ = syscall.Unmount(state, syscall.MNT_DETACH) // discard a stale service mount
	if e = os.MkdirAll(state, 0700); e != nil {
		return e
	}
	if e = syscall.Mount("/datastore/data0/chrony", state, "", syscall.MS_BIND, ""); e != nil {
		return e
	}
	defer syscall.Unmount(state, syscall.MNT_DETACH)
	// No network command socket or NTP server; control is local Unix socket only.
	text := "driftfile /run/fractal-time-state/drift\npidfile /run/chrony/chronyd.pid\nbindcmdaddress /run/chrony/chronyd.sock\ncmdport 0\nport 0\nrtcsync\nmakestep 1 3\n"
	for _, s := range c.NTP.Servers {
		text += "server " + s + " iburst\n"
	}
	if e = config.AtomicWrite("/run/chrony.conf", []byte(text), 0600); e != nil {
		return e
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(signals)
	p := exec.Command("/usr/sbin/chronyd", "-d", "-u", "chronyd", "-F", "1", "-f", "/run/chrony.conf")
	p.Stdout = os.Stdout
	p.Stderr = os.Stderr
	if e = p.Start(); e != nil {
		return e
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	defer func() {
		_ = p.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = p.Process.Kill()
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-signals:
			return nil
		case e := <-done:
			return fmt.Errorf("chronyd exited: %v", e)
		case <-deadline.C:
			return fmt.Errorf("chronyd control socket did not become ready")
		case <-ticker.C:
			if st, e := os.Stat("/run/chrony/chronyd.sock"); e == nil && st.Mode()&os.ModeSocket != 0 {
				notifyReady()
				select {
				case <-signals:
					return nil
				case e := <-done:
					return fmt.Errorf("chronyd exited: %v", e)
				}
			}
		}
	}
}
