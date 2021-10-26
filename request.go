package tcp

import (
	"go.unistack.org/micro/v3/codec"
	"go.unistack.org/micro/v3/metadata"
	"go.unistack.org/micro/v3/server"
)

var _ server.Request = &tcpRequest{}

type tcpRequest struct {
	codec       codec.Codec
	body        interface{}
	header      map[string]string
	method      string
	endpoint    string
	contentType string
	service     string
}

func (r *tcpRequest) Service() string {
	return r.service
}

func (r *tcpRequest) Method() string {
	return r.method
}

func (r *tcpRequest) Endpoint() string {
	return r.endpoint
}

func (r *tcpRequest) ContentType() string {
	return r.contentType
}

func (r *tcpRequest) Header() metadata.Metadata {
	return r.header
}

func (r *tcpRequest) Body() interface{} {
	return r.body
}

func (r *tcpRequest) Read() ([]byte, error) {
	return nil, nil
}

func (r *tcpRequest) Stream() bool {
	return false
}

func (r *tcpRequest) Codec() codec.Codec {
	return r.codec
}
