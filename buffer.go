package tcp

import (
	"bytes"
)

type buffer struct {
	*bytes.Buffer
}

func (b *buffer) Close() error {
	b.Buffer.Reset()
	return nil
}
