package core

import (
	"net"
	"syscall"
)

// Windows otherwise permits overlapping wildcard/specific-address binds.
// SO_EXCLUSIVEADDRUSE must be set before bind, so an occupied port cannot
// appear available while traffic continues to reach a different process.
func listenerConfig() net.ListenConfig {
	const soExclusiveAddrUse = -5
	return net.ListenConfig{Control: func(network, address string, c syscall.RawConn) error {
		var socketErr error
		err := c.Control(func(fd uintptr) {
			socketErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, soExclusiveAddrUse, 1)
		})
		if err != nil {
			return err
		}
		return socketErr
	}}
}
