package grpcproxy

import (
	"bytes"
	"testing"
)

// TestProxyMessageUnmarshalCopies guards the copy invariant: grpc-go v1.80.0 pools
// receive buffers over 1024 B and Free()s them the instant Unmarshal returns, so the
// slice handed to Unmarshal aliases memory the pool will reuse. If Unmarshal merely
// aliased it (p.data = bytes), overwriting the source afterwards would corrupt the
// forwarded payload. This fails against the old aliasing version and passes against
// message.go's append-copy.
func TestProxyMessageUnmarshalCopies(t *testing.T) {
	// > 1024 B so it would be pool-backed in production.
	src := make([]byte, 4096)
	for i := range src {
		src[i] = byte(i)
	}
	want := append([]byte(nil), src...)

	var p proxyMessage
	if err := p.Unmarshal(src); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Backing arrays must be distinct — proves a copy regardless of payload size.
	if len(p.data) > 0 && len(src) > 0 && &p.data[0] == &src[0] {
		t.Fatal("p.data aliases the source buffer; Unmarshal did not copy")
	}

	// Simulate the pool freeing/reusing the buffer under us.
	for i := range src {
		src[i] = 0xFF
	}

	if !bytes.Equal(p.data, want) {
		t.Fatalf("p.data was clobbered when source buffer was reused: got %x..., want %x...",
			p.data[:8], want[:8])
	}
}

// TestProxyMessageUnmarshalReset mirrors the proxy loop (proxy.go), which uses a fresh
// proxyMessage per stream iteration and relies on Reset. A second Unmarshal must return
// its own payload with no cross-message aliasing.
func TestProxyMessageUnmarshalReset(t *testing.T) {
	first := bytes.Repeat([]byte{0xAA}, 2048)
	second := bytes.Repeat([]byte{0xBB}, 1536)

	var p proxyMessage
	if err := p.Unmarshal(first); err != nil {
		t.Fatalf("Unmarshal first: %v", err)
	}
	firstOut, err := p.Marshal()
	if err != nil {
		t.Fatalf("Marshal first: %v", err)
	}
	if !bytes.Equal(firstOut, first) {
		t.Fatal("first payload not preserved")
	}

	p.Reset()
	if p.data != nil {
		t.Fatal("Reset did not clear data")
	}

	if err := p.Unmarshal(second); err != nil {
		t.Fatalf("Unmarshal second: %v", err)
	}
	secondOut, err := p.Marshal()
	if err != nil {
		t.Fatalf("Marshal second: %v", err)
	}
	if !bytes.Equal(secondOut, second) {
		t.Fatal("second payload not preserved after Reset")
	}
}

// BenchmarkProxyMessageUnmarshal documents the per-message copy cost on the proxy hot
// path. A non-zero alloc here is inherent and correct: copying the pooled buffer is what
// prevents corruption — this benchmark measures that copy rather than asserting zero allocs.
func BenchmarkProxyMessageUnmarshal(b *testing.B) {
	src := make([]byte, 4096)
	for i := range src {
		src[i] = byte(i)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for i := 0; i < b.N; i++ {
		var p proxyMessage
		if err := p.Unmarshal(src); err != nil {
			b.Fatal(err)
		}
	}
}
