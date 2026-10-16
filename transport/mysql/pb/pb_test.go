package pb

import "testing"

func TestServiceName(t *testing.T) {
	if n := _MySql_serviceDesc.ServiceName; n != "MySql" {
		t.Fatalf("service name %q, want %q", n, "MySql")
	}
}
