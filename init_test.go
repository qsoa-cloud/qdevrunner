package main

import (
	"testing"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"

	"gopkg.qsoa.cloud/grpccodec"
	"gopkg.qsoa.cloud/grpccodec/inittest"
)

func TestInit(t *testing.T) {
	inittest.Check(t)
	if encoding.GetCodecV2(proto.Name) != grpccodec.Codec {
		t.Fatal("gopkg.qsoa.cloud/grpccodec/register is not imported")
	}
}
