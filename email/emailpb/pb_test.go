package emailpb

import "testing"

func TestServiceName(t *testing.T) {
	if n := _QEmail_serviceDesc.ServiceName; n != "QEmail" {
		t.Fatalf("service name %q, want %q", n, "QEmail")
	}
}
