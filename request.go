package tcp

import (
	"github.com/unistack-org/micro/v3/codec"
)

type tcpRequest struct {
	service     string
	method      string
	endpoint    string
	contentType string
	header      map[string]string
	body        interface{}
	codec       codec.Reader
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

func (r *tcpRequest) Header() map[string]string {
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

func (r *tcpRequest) Codec() codec.Reader {
	return r.codec
}
