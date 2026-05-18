package prometheus

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"

	"gopkg.qsoa.cloud/qdevrunner/metricsstore"
)

// ToService scrapes Prometheus metrics from a service's prometheus.sock.
type ToService struct {
	service    string
	addr       string
	store      *metricsstore.Store
	client     *http.Client
	ready      bool
	readyErr   error
}

func New(service, addr string, store *metricsstore.Store) *ToService {
	return &ToService{
		service: service,
		addr:    addr,
		store:   store,
	}
}

func (t *ToService) Prepare() error {
	return nil
}

func (t *ToService) Run(ctx context.Context) {
	t.client = &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return net.DialTimeout("unix", t.addr, 5*time.Second)
			},
		},
		Timeout: 10 * time.Second,
	}

	// Wait for socket to be available.
	for ctx.Err() == nil {
		conn, err := net.DialTimeout("unix", t.addr, time.Second)
		if err == nil {
			conn.Close()
			t.ready = true
			break
		}
		t.readyErr = err
		time.Sleep(100 * time.Millisecond)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.scrape()
		}
	}
}

func (t *ToService) IsReady() bool {
	return t.ready
}

func (t *ToService) IsReadyError() error {
	return t.readyErr
}

func (t *ToService) scrape() {
	resp, err := t.client.Get("http://unix/metrics")
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body)
		return
	}

	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(resp.Body)
	if err != nil {
		log.Printf("prometheus scrape parse error: %v", err)
		return
	}

	var metrics []metricsstore.MetricValue
	for name, fam := range families {
		for _, m := range fam.GetMetric() {
			labels := make(map[string]string)
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}

			switch fam.GetType() {
			case dto.MetricType_COUNTER:
				metrics = append(metrics, metricsstore.MetricValue{
					Name:   name,
					Type:   metricsstore.MetricCounter,
					Labels: labels,
					Value:  m.GetCounter().GetValue(),
				})
			case dto.MetricType_GAUGE:
				metrics = append(metrics, metricsstore.MetricValue{
					Name:   name,
					Type:   metricsstore.MetricGauge,
					Labels: labels,
					Value:  m.GetGauge().GetValue(),
				})
			case dto.MetricType_SUMMARY:
				metrics = append(metrics, metricsstore.MetricValue{
					Name:   name,
					Type:   metricsstore.MetricSummary,
					Labels: labels,
					Sum:    m.GetSummary().GetSampleSum(),
					Count:  m.GetSummary().GetSampleCount(),
				})
			case dto.MetricType_HISTOGRAM:
				metrics = append(metrics, metricsstore.MetricValue{
					Name:   fmt.Sprintf("%s_sum", name),
					Type:   metricsstore.MetricSummary,
					Labels: labels,
					Sum:    m.GetHistogram().GetSampleSum(),
					Count:  m.GetHistogram().GetSampleCount(),
				})
			case dto.MetricType_UNTYPED:
				metrics = append(metrics, metricsstore.MetricValue{
					Name:   name,
					Type:   metricsstore.MetricGauge,
					Labels: labels,
					Value:  m.GetUntyped().GetValue(),
				})
			}
		}
	}

	if len(metrics) > 0 {
		t.store.Add(metricsstore.Snapshot{
			Service:   t.service,
			Timestamp: time.Now(),
			Metrics:   metrics,
		})
	}
}

