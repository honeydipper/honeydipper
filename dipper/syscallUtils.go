package dipper

import (
	"syscall"
)

// FdSet : push a fd into a bitmap data structure that stores all the fds for select syscall
func FdSet(p *syscall.FdSet, i int) {
	p.Bits[i/64] |= 1 << uint(i) % 64
}

// FdIsSet : check if the fd is in the fdset
func FdIsSet(p *syscall.FdSet, i int) bool {
	return (p.Bits[i/64] & (1 << uint(i) % 64)) != 0
}

// FdZero : wipe all the fds from the FdSet
func FdZero(p *syscall.FdSet) {
	for i := range p.Bits {
		p.Bits[i] = 0
	}
}
