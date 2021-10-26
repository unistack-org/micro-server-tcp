package tcp

import (
	"go.unistack.org/micro/v3/codec"
	"go.unistack.org/micro/v3/metadata"
	"go.unistack.org/micro/v3/server"
)

var _ server.Message = &tcpMessage{}

type tcpMessage struct {
	payload     interface{}
	codec       codec.Codec
	header      metadata.Metadata
	topic       string
	contentType string
	body        []byte
}

func (r *tcpMessage) Topic() string {
	return r.topic
}

func (r *tcpMessage) Payload() interface{} {
	return r.payload
}

func (r *tcpMessage) ContentType() string {
	return r.contentType
}

func (r *tcpMessage) Header() metadata.Metadata {
	return r.header
}

func (r *tcpMessage) Body() []byte {
	return r.body
}

func (r *tcpMessage) Codec() codec.Codec {
	return r.codec
}
