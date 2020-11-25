package tcp

import (
	"github.com/unistack-org/micro/v3/codec"
	"github.com/unistack-org/micro/v3/metadata"
)

type tcpMessage struct {
	topic       string
	payload     interface{}
	contentType string
	header      metadata.Metadata
	body        []byte
	codec       codec.Codec
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
