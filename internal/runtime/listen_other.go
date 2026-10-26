//go:build !windows

package runtime

import "net"

func listenTCP(address string) (net.Listener, error) {
	return net.Listen("tcp", address)
}
