//go:generate protoc -I secretspb --gogofaster_out=plugins=grpc:secretspb secretspb/qsecrets.proto

package secrets
