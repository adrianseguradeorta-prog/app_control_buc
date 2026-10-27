//go:build !windows

package core

import "net"

func listenerConfig() net.ListenConfig { return net.ListenConfig{} }
