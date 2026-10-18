package secrets

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gopkg.qsoa.cloud/qdevrunner/secrets/secretspb"
)

func TestGet(t *testing.T) {
	store := map[string]map[string]string{
		"api": {"db/password": "api-value"},
		"web": {"token": "web-value"},
	}
	get := func(service, path string) (string, bool) {
		v, ok := store[service][path]
		return v, ok
	}
	fr := NewFromRunner("", "api", get)

	resp, err := fr.Get(context.Background(), &secretspb.GetSecretReq{Path: "db/password"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if resp.Value != "api-value" {
		t.Errorf("got %q, want %q", resp.Value, "api-value")
	}

	_, err = fr.Get(context.Background(), &secretspb.GetSecretReq{Path: "token"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	if status.Convert(err).Message() != "secret not found" {
		t.Errorf("message %q", status.Convert(err).Message())
	}
}
