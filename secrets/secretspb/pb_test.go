package secretspb

import "testing"

func TestServiceName(t *testing.T) {
	if n := _QSecrets_serviceDesc.ServiceName; n != "QSecrets" {
		t.Fatalf("service name %q, want %q", n, "QSecrets")
	}
}
