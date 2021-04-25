package tcp

import (
	"net"

	"github.com/unistack-org/micro/v3/register"
	"github.com/unistack-org/micro/v3/server"
)

type Handler interface {
	Serve(net.Conn)
}

type tcpHandler struct {
	opts       server.HandlerOptions
	hd         interface{}
	eps        []*register.Endpoint
	maxMsgSize int
}

func (h *tcpHandler) Name() string {
	return "handler"
}

func (h *tcpHandler) Handler() interface{} {
	return h.hd
}

func (h *tcpHandler) Endpoints() []*register.Endpoint {
	return h.eps
}

func (h *tcpHandler) Options() server.HandlerOptions {
	return h.opts
}
