//go:build !darwin && !linux

package core

import "net"

func peerPID(net.Conn) int { return 0 }
