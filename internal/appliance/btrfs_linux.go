package appliance

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"fractalcr/internal/config"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Linux Btrfs UAPI, x86-64. The image is built for this architecture. Byte
// buffers preserve the kernel ABI including reserved fields and padding.
const (
	btrfsSnapshotCreate  = 0x50009417
	btrfsSnapshotDelete  = 0x5000943f
	btrfsSubvolumeFlags  = 0x80089419
	btrfsFSInfo          = 0x8400941f
	btrfsDeviceInfo      = 0xd000941e
	btrfsScrub           = 0xc400941b
	btrfsScrubCancel     = 0x941c
	btrfsScrubProgress   = 0xc400941d
	btrfsBalance         = 0xc4009420
	btrfsBalanceControl  = 0x40049421
	btrfsBalanceProgress = 0x84009422
)

var datastoreDirectory = "/datastore/data0"

func btrfsIOCTL(fd uintptr, operation uintptr, buffer []byte) error {
	var errno syscall.Errno
	if len(buffer) == 0 {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, operation, 0)
	} else {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, operation, uintptr(unsafe.Pointer(&buffer[0])))
	}
	if errno != 0 {
		return errno
	}
	return nil
}

func openDatastore() (*os.File, error) {
	fd, err := syscall.Open(datastoreDirectory, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("datastore data0 is offline")
	}
	f := os.NewFile(uintptr(fd), "datastore data0")
	var st syscall.Statfs_t
	if err = syscall.Fstatfs(fd, &st); err != nil || uint64(st.Type) != 0x9123683e {
		f.Close()
		return nil, fmt.Errorf("datastore data0 is not a mounted Btrfs filesystem")
	}
	return f, nil
}

func snapshotOperation(action, name string) (string, error) {
	if name != "" && !config.Name.MatchString(name) {
		return "", fmt.Errorf("invalid snapshot name")
	}
	root, err := openDatastore()
	if err != nil {
		return "", err
	}
	defer root.Close()
	if err = syscall.Mkdirat(int(root.Fd()), ".snapshots", 0700); err != nil && err != syscall.EEXIST {
		return "", err
	}
	fd, err := syscall.Openat(int(root.Fd()), ".snapshots", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return "", fmt.Errorf("snapshot directory is unavailable")
	}
	dir := os.NewFile(uintptr(fd), "snapshot directory")
	defer dir.Close()
	if action == "list" {
		entries, err := dir.ReadDir(-1)
		if err != nil {
			return "", err
		}
		names := map[string]string{}
		for _, entry := range entries {
			if !entry.IsDir() || !config.Name.MatchString(entry.Name()) {
				continue
			}
			child, err := syscall.Openat(fd, entry.Name(), syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
			if err != nil {
				continue
			}
			var stat syscall.Stat_t
			flags := make([]byte, 8)
			if syscall.Fstat(child, &stat) == nil && stat.Ino == 256 && btrfsIOCTL(uintptr(child), btrfsSubvolumeFlags, flags) == nil {
				access := "read-write"
				if binary.LittleEndian.Uint64(flags)&2 != 0 {
					access = "read-only"
				}
				names[entry.Name()] = access
			}
			syscall.Close(child)
		}
		lines := []string{"Snapshot                         Access"}
		for _, name := range config.Keys(names) {
			lines = append(lines, fmt.Sprintf("%-32s %s", name, names[name]))
		}
		return strings.Join(lines, "\n"), nil
	}
	buffer := make([]byte, 4096)
	copy(buffer[56:], name)
	switch action {
	case "create":
		binary.LittleEndian.PutUint64(buffer, uint64(root.Fd()))
		binary.LittleEndian.PutUint64(buffer[16:], 2)
		err = btrfsIOCTL(dir.Fd(), btrfsSnapshotCreate, buffer)
	case "delete":
		err = btrfsIOCTL(dir.Fd(), btrfsSnapshotDelete, buffer)
	default:
		return "", fmt.Errorf("invalid snapshot operation")
	}
	if err != nil {
		return "", fmt.Errorf("snapshot %s: %w", action, err)
	}
	if err = dir.Sync(); err != nil {
		return "", err
	}
	return fmt.Sprintf("Snapshot %s: %s completed.", name, action), nil
}

func btrfsDevices(f *os.File) ([]uint64, error) {
	info := make([]byte, 1024)
	if err := btrfsIOCTL(f.Fd(), btrfsFSInfo, info); err != nil {
		return nil, err
	}
	max, count := binary.LittleEndian.Uint64(info), binary.LittleEndian.Uint64(info[8:])
	if max > 65536 || count > 256 {
		return nil, fmt.Errorf("datastore device inventory exceeds management bounds")
	}
	devices := []uint64{}
	for id := uint64(1); id <= max; id++ {
		data := make([]byte, 4096)
		binary.LittleEndian.PutUint64(data, id)
		err := btrfsIOCTL(f.Fd(), btrfsDeviceInfo, data)
		if err == syscall.ENODEV {
			continue
		}
		if err != nil {
			return nil, err
		}
		devices = append(devices, id)
	}
	if len(devices) == 0 || uint64(len(devices)) != count {
		return nil, fmt.Errorf("datastore device inventory is incomplete")
	}
	return devices, nil
}

type maintenanceJob struct {
	Kind          string `json:"kind"`
	State         string `json:"state"`
	Started       string `json:"started"`
	Finished      string `json:"finished,omitempty"`
	Error         string `json:"error,omitempty"`
	Bytes         uint64 `json:"bytes,omitempty"`
	Corrected     uint64 `json:"corrected,omitempty"`
	Uncorrectable uint64 `json:"uncorrectable,omitempty"`
}

var maintenance struct {
	sync.Mutex
	jobs   map[string]maintenanceJob
	cancel bool
}

func saveMaintenance(job maintenanceJob) error {
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}
	return config.AtomicWrite(filepath.Join(datastoreDirectory, "config", "maintenance-"+job.Kind+".json"), data, 0600)
}

func startMaintenance(kind string) (string, error) {
	f, err := openDatastore()
	if err != nil {
		return "", err
	}
	devices, err := btrfsDevices(f)
	if err != nil {
		f.Close()
		return "", err
	}
	maintenance.Lock()
	defer maintenance.Unlock()
	for _, job := range maintenance.jobs {
		if job.State == "running" || job.State == "canceling" {
			f.Close()
			return "", fmt.Errorf("a datastore maintenance operation is already running")
		}
	}
	job := maintenanceJob{Kind: kind, State: "running", Started: time.Now().UTC().Format(time.RFC3339)}
	if err = saveMaintenance(job); err != nil {
		f.Close()
		return "", err
	}
	if maintenance.jobs == nil {
		maintenance.jobs = map[string]maintenanceJob{}
	}
	maintenance.jobs[kind] = job
	maintenance.cancel = false
	go func() {
		defer f.Close()
		var failure error
		if kind == "balance" {
			buffer := make([]byte, 1024)
			binary.LittleEndian.PutUint64(buffer, 7)
			failure = btrfsIOCTL(f.Fd(), btrfsBalance, buffer)
		} else {
			for _, id := range devices {
				maintenance.Lock()
				canceled := maintenance.cancel
				maintenance.Unlock()
				if canceled {
					failure = syscall.ECANCELED
					break
				}
				buffer := make([]byte, 1024)
				binary.LittleEndian.PutUint64(buffer, id)
				binary.LittleEndian.PutUint64(buffer[16:], ^uint64(0))
				failure = btrfsIOCTL(f.Fd(), btrfsScrub, buffer)
				job.Bytes += binary.LittleEndian.Uint64(buffer[48:]) + binary.LittleEndian.Uint64(buffer[56:])
				job.Corrected += binary.LittleEndian.Uint64(buffer[128:])
				job.Uncorrectable += binary.LittleEndian.Uint64(buffer[120:])
				if failure != nil {
					break
				}
			}
		}
		job.State = "completed"
		if failure != nil {
			job.State = "failed"
			job.Error = failure.Error()
			if errors.Is(failure, syscall.ECANCELED) {
				job.State = "canceled"
			}
		}
		if job.Uncorrectable > 0 {
			job.State = "failed"
			job.Error = "uncorrectable filesystem errors detected"
		}
		job.Finished = time.Now().UTC().Format(time.RFC3339)
		maintenance.Lock()
		defer maintenance.Unlock()
		if err := saveMaintenance(job); err != nil {
			job.State = "failed"
			job.Error = "cannot persist maintenance result"
		}
		maintenance.jobs[kind] = job
	}()
	return "Datastore data0: " + kind + " started.", nil
}

func maintenanceOperation(kind, action string) (string, error) {
	if kind != "scrub" && kind != "balance" {
		return "", fmt.Errorf("invalid maintenance operation")
	}
	if action == "start" {
		return startMaintenance(kind)
	}
	f, err := openDatastore()
	if err != nil {
		return "", err
	}
	defer f.Close()
	if action == "cancel" {
		maintenance.Lock()
		defer maintenance.Unlock()
		if kind == "scrub" {
			err = btrfsIOCTL(f.Fd(), btrfsScrubCancel, nil)
		} else {
			_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), btrfsBalanceControl, 2)
			if errno != 0 {
				err = errno
			}
		}
		if err != nil && err != syscall.ENOTCONN {
			return "", fmt.Errorf("%s cancel: %w", kind, err)
		}
		maintenance.cancel = true
		if job, ok := maintenance.jobs[kind]; ok && job.State == "running" {
			job.State = "canceling"
			maintenance.jobs[kind] = job
		}
		return "Datastore data0: " + kind + " cancellation requested.", nil
	}
	maintenance.Lock()
	job, ok := maintenance.jobs[kind]
	maintenance.Unlock()
	if !ok {
		data, e := os.ReadFile(filepath.Join(datastoreDirectory, "config", "maintenance-"+kind+".json"))
		if os.IsNotExist(e) {
			return "No recorded " + kind + " operation.", nil
		}
		if e != nil {
			return "", e
		}
		if err = json.Unmarshal(data, &job); err != nil {
			return "", err
		}
		if job.State == "running" || job.State == "canceling" {
			job.State = "interrupted by management restart"
		}
	}
	return fmt.Sprintf("Datastore data0 %s\n  State: %s\n  Started: %s\n  Finished: %s\n  Bytes checked: %d\n  Corrected errors: %d\n  Uncorrectable errors: %d\n  Detail: %s\n", kind, job.State, job.Started, job.Finished, job.Bytes, job.Corrected, job.Uncorrectable, displayText(job.Error)), nil
}
