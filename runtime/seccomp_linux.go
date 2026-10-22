//go:build linux

package runtime

import (
	"encoding/binary"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ---------------------------------------------------------------
// Seccomp-BPF Syscall Filter
// ---------------------------------------------------------------
// This implements a Berkeley Packet Filter (BPF) program that blocks
// dangerous syscalls inside containers. This is the same mechanism
// Docker uses (via libseccomp) but implemented from scratch in Go.
//
// Blocked syscalls: reboot, kexec_load, init_module, delete_module,
//   mount, umount2, swapon, swapoff, pivot_root, acct, settimeofday,
//   clock_settime, sethostname, setdomainname
//
// Academic reference:
//   Drewry, W. (2012). "Seccomp filter" [Linux kernel documentation]
//   https://www.kernel.org/doc/Documentation/prctl/seccomp_filter.txt
// ---------------------------------------------------------------

const (
	SECCOMP_RET_ERRNO = 0x00050000 // Return -1 and set errno to EPERM
	SECCOMP_RET_ALLOW = 0x7fff0000 // Allow the syscall

	BPF_LD  = 0x00
	BPF_W   = 0x00
	BPF_ABS = 0x20
	BPF_JMP = 0x05
	BPF_JEQ = 0x10
	BPF_K   = 0x00
	BPF_RET = 0x06

	// Audit architecture for x86_64
	AUDIT_ARCH_X86_64 = 0xC000003E
)

// Syscall numbers for x86_64 Linux
var blockedSyscalls = []uint32{
	169, // reboot
	246, // kexec_load
	175, // init_module
	176, // delete_module
	165, // mount (within container context)
	166, // umount2
	167, // swapon
	168, // swapoff
	155, // pivot_root
	163, // acct
	164, // settimeofday
	227, // clock_settime
	170, // sethostname
	171, // setdomainname
}

// bpfInstruction matches the kernel's struct sock_filter
type bpfInstruction struct {
	Code uint16
	Jt   uint8
	Jf   uint8
	K    uint32
}

// buildSeccompFilter constructs a BPF program that blocks dangerous syscalls.
// Returns the number of rules applied.
func buildSeccompFilter() ([]bpfInstruction, int) {
	prog := []bpfInstruction{}

	// Load the syscall number (offset 0 in seccomp_data)
	prog = append(prog, bpfInstruction{
		Code: BPF_LD | BPF_W | BPF_ABS,
		K:    0, // offsetof(struct seccomp_data, nr)
	})

	// For each blocked syscall, add a JEQ check
	numBlocked := len(blockedSyscalls)
	for i, nr := range blockedSyscalls {
		remaining := numBlocked - i - 1
		prog = append(prog, bpfInstruction{
			Code: BPF_JMP | BPF_JEQ | BPF_K,
			Jt:   uint8(remaining + 1), // Jump to ERRNO return
			Jf:   0,                     // Continue checking
			K:    nr,
		})
	}

	// Default: allow
	prog = append(prog, bpfInstruction{
		Code: BPF_RET | BPF_K,
		K:    SECCOMP_RET_ALLOW,
	})

	// Blocked: return EPERM
	prog = append(prog, bpfInstruction{
		Code: BPF_RET | BPF_K,
		K:    SECCOMP_RET_ERRNO | 1, // EPERM = 1
	})

	return prog, numBlocked
}

// ApplySeccompFilter installs the BPF filter on the current process.
// Must be called after fork but before exec in the child process.
func ApplySeccompFilter() (int, error) {
	prog, ruleCount := buildSeccompFilter()

	// Set NO_NEW_PRIVS (required for unprivileged seccomp)
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return 0, err
	}

	// Construct the sock_fprog struct
	rawProg := make([]byte, len(prog)*8)
	for i, inst := range prog {
		binary.LittleEndian.PutUint16(rawProg[i*8:], inst.Code)
		rawProg[i*8+2] = inst.Jt
		rawProg[i*8+3] = inst.Jf
		binary.LittleEndian.PutUint32(rawProg[i*8+4:], inst.K)
	}

	// sock_fprog: { unsigned short len; struct sock_filter *filter; }
	type sockFprog struct {
		Len    uint16
		_      [6]byte // padding for alignment
		Filter uintptr
	}

	fprog := sockFprog{
		Len:    uint16(len(prog)),
		Filter: uintptr(unsafe.Pointer(&rawProg[0])),
	}

	// SECCOMP_SET_MODE_FILTER = 1
	_, _, errno := unix.RawSyscall(unix.SYS_SECCOMP, 1, 0, uintptr(unsafe.Pointer(&fprog)))
	if errno != 0 {
		return 0, errno
	}

	return ruleCount, nil
}
