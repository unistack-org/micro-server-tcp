package tcp

import "github.com/unistack-org/micro/v3/codec"

type tcpMessage struct {
	topic       string
	payload     interface{}
	contentType string
	header      map[string]string
	body        []byte
	codec       codec.Reader
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

func (r *tcpMessage) Header() map[string]string {
	return r.header
}

func (r *tcpMessage) Body() []byte {
	return r.body
}

func (r *tcpMessage) Codec() codec.Reader {
	return r.codec
}
