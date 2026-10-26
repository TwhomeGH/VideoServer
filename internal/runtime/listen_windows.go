package runtime

import (
	"context"
	"net"
	"syscall"
)

// listenTCP 使用 Windows 獨占綁定，避免其他可重用 socket 與本服務共享端口。
func listenTCP(address string) (net.Listener, error) {
	config := net.ListenConfig{Control: func(network, address string, raw syscall.RawConn) error {
		var optionError error
		err := raw.Control(func(socket uintptr) {
			const exclusiveAddressUse = ^syscall.SO_REUSEADDR
			optionError = syscall.SetsockoptInt(syscall.Handle(socket), syscall.SOL_SOCKET, exclusiveAddressUse, 1)
		})
		if err != nil {
			return err
		}
		return optionError
	}}
	return config.Listen(context.Background(), "tcp", address)
}
