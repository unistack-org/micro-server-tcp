package tcp

import (
	"net"

	"github.com/unistack-org/micro/v3/registry"
	"github.com/unistack-org/micro/v3/server"
)

type Handler interface {
	Serve(net.Conn)
}

type tcpHandler struct {
	opts       server.HandlerOptions
	eps        []*registry.Endpoint
	hd         interface{}
	maxMsgSize int
}

func (h *tcpHandler) Name() string {
	return "handler"
}

func (h *tcpHandler) Handler() interface{} {
	return h.hd
}

func (h *tcpHandler) Endpoints() []*registry.Endpoint {
	return h.eps
}

func (h *tcpHandler) Options() server.HandlerOptions {
	return h.opts
}
