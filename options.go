package tcp

import (
	"crypto/tls"
	"net"

	"github.com/unistack-org/micro/v3/server"
)

// DefaultMaxMsgSize define maximum message size that server can send
// or receive.  Default value is 8K
var DefaultMaxMsgSize = 1024 * 8

type (
	maxMsgSizeKey struct{}
	tlsAuth       struct{}
	maxConnKey    struct{}
	netListener   struct{}
)

//
// MaxMsgSize set the maximum message in bytes the server can receive and
// send.  Default maximum message size is 8K
//
func MaxMsgSize(s int) server.Option {
	return server.SetOption(maxMsgSizeKey{}, s)
}

// AuthTLS should be used to setup a secure authentication using TLS
func AuthTLS(t *tls.Config) server.Option {
	return server.SetOption(tlsAuth{}, t)
}

// MaxConn specifies maximum number of max simultaneous connections to server
func MaxConn(n int) server.Option {
	return server.SetOption(maxConnKey{}, n)
}

// Listener specifies the net.Listener to use instead of the default
func Listener(l net.Listener) server.Option {
	return server.SetOption(netListener{}, l)
}
