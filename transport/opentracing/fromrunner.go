package opentracing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"time"

	"github.com/qsoa-cloud/qdevrunner/tracer"
	"github.com/qsoa-cloud/qdevrunner/transport"
)

type OpenTracing struct {
	service string
	addr    string
	store   *tracer.Store
}

func New(service, addr string, store *tracer.Store) *OpenTracing {
	return &OpenTracing{
		service: service,
		addr:    addr,
		store:   store,
	}
}

func (t *OpenTracing) Serve(ctx context.Context) error {
	f, err := transport.CreatePipeBlocking(ctx, t.addr)
	if err != nil {
		return err
	}
	defer f.Close()

	j := json.NewDecoder(f)

	for ctx.Err() == nil {
		var span tracer.Span
		if err := j.Decode(&span); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			log.Printf("Cannot read tracer JSON: %s", err.Error())
			continue
		}

		t.store.Add(tracer.SpanRecord{
			Service:    t.service,
			ReceivedAt: time.Now(),
			Span:       span,
		})
	}

	return nil
}
