package grpcproxy

type proxyMessage struct {
	data []byte
}

func (p *proxyMessage) Marshal() ([]byte, error) {
	return p.data, nil
}

func (p *proxyMessage) Unmarshal(bytes []byte) error {
	// gRPC's default codec materializes incoming messages into a buffer it
	// pulls from a pool when the payload exceeds mem.bufferPoolingThreshold
	// (1024 B). That buffer is Free()'d as soon as codec.Unmarshal returns,
	// so the slice we get here aliases memory that will be reused by the
	// next pool consumer. We must copy.
	p.data = append(p.data[:0], bytes...)
	return nil
}

func (p *proxyMessage) Reset() {
	p.data = nil
}

func (p *proxyMessage) String() string {
	return "proxyMessage"
}

func (p *proxyMessage) ProtoMessage() {}
