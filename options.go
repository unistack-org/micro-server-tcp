package tcp

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/unistack-org/micro/v3/broker"
	"github.com/unistack-org/micro/v3/codec"
	"github.com/unistack-org/micro/v3/registry"
	"github.com/unistack-org/micro/v3/server"
)

var (
	// DefaultMaxMsgSize define maximum message size that server can send
	// or receive.  Default value is 8K
	DefaultMaxMsgSize = 1024 * 8
)

type maxMsgSizeKey struct{}
type tlsAuth struct{}
type maxConnKey struct{}
type netListener struct{}

//
// MaxMsgSize set the maximum message in bytes the server can receive and
// send.  Default maximum message size is 8K
//
func MaxMsgSize(s int) server.Option {
	return setServerOption(maxMsgSizeKey{}, s)
}

// AuthTLS should be used to setup a secure authentication using TLS
func AuthTLS(t *tls.Config) server.Option {
	return setServerOption(tlsAuth{}, t)
}

// MaxConn specifies maximum number of max simultaneous connections to server
func MaxConn(n int) server.Option {
	return setServerOption(maxConnKey{}, n)
}

// Listener specifies the net.Listener to use instead of the default
func Listener(l net.Listener) server.Option {
	return setServerOption(netListener{}, l)
}

func newOptions(opt ...server.Option) server.Options {
	opts := server.Options{
		Codecs:   make(map[string]codec.NewCodec),
		Metadata: map[string]string{},
		Context:  context.Background(),
	}

	for _, o := range opt {
		o(&opts)
	}

	if opts.Broker == nil {
		opts.Broker = broker.DefaultBroker
	}

	if opts.Registry == nil {
		opts.Registry = registry.DefaultRegistry
	}

	if len(opts.Address) == 0 {
		opts.Address = server.DefaultAddress
	}

	if len(opts.Name) == 0 {
		opts.Name = server.DefaultName
	}

	if len(opts.Id) == 0 {
		opts.Id = server.DefaultId
	}

	if len(opts.Version) == 0 {
		opts.Version = server.DefaultVersion
	}

	return opts
}
