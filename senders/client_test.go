package senders_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wavefronthq/wavefront-sdk-go/histogram"
	"github.com/wavefronthq/wavefront-sdk-go/senders"
)

const (
	wfPort    = "8080"
	proxyPort = "8081"
	token     = "DUMMY_TOKEN"
)

func TestMain(m *testing.M) {
	wf := http.Server{Addr: "localhost:" + wfPort}
	proxy := http.Server{Addr: "localhost:" + proxyPort}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Host, wfPort) {
			if strings.HasSuffix(r.Header.Get("Authorization"), token) {
				b, err := ioutil.ReadAll(r.Body)
				if err != nil {
					log.Fatalln(err)
				}
				defer r.Body.Close()
				if r.Header.Get("Content-Type") == "application/octet-stream" {
					gr, err := gzip.NewReader(bytes.NewBuffer(b))
					defer gr.Close()
					data, err := ioutil.ReadAll(gr)
					if err != nil {
						log.Fatalln(err)
					}
					println("GUNZIPPED: " + string(data))
				} else {
					println("PLAINTXT: " + string(b))
				}
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		if strings.HasSuffix(r.Host, proxyPort) {
			if len(r.Header.Get("Authorization")) == 0 {
				w.WriteHeader(http.StatusOK)
				return
			}
		}

		w.WriteHeader(http.StatusForbidden)
	})
	go func() { wf.ListenAndServe() }()
	go func() { proxy.ListenAndServe() }()

	exitVal := m.Run()

	wf.Shutdown(context.Background())
	proxy.Shutdown(context.Background())

	os.Exit(exitVal)
}

func TestSendDirect(t *testing.T) {
	wf, err := senders.NewSender("http://" + token + "@localhost:" + wfPort)
	require.NoError(t, err)
	doTest(t, wf)
}

func TestSendDirectWithTags(t *testing.T) {
	tags := map[string]string{"foo": "bar"}
	wf, err := senders.NewSender("http://"+token+"@localhost:"+wfPort, senders.SDKMetricsTags(tags))
	require.NoError(t, err)
	doTest(t, wf)
}

func TestSendProxy(t *testing.T) {
	wf, err := senders.NewSender("http://localhost:" + proxyPort)
	require.NoError(t, err)
	doTest(t, wf)
}

func doTest(t *testing.T, wf senders.Sender) {
	if err := wf.SendMetric("new-york.power.usage", 42422.0, 0, "go_test", map[string]string{"env": "test"}); err != nil {
		t.Error("Failed SendMetric", err)
	}

	centroids := []histogram.Centroid{
		{Value: 30.0, Count: 20},
		{Value: 5.1, Count: 10},
	}

	hgs := map[histogram.Granularity]bool{
		histogram.MINUTE: true,
		histogram.HOUR:   true,
		histogram.DAY:    true,
	}

	if err := wf.SendDistribution("request.latency", centroids, hgs, 0, "appServer1", map[string]string{"region": "us-west"}); err != nil {
		t.Error("Failed SendDistribution", err)
	}

	if err := wf.SendSpan("getAllUsers", 0, 343500, "localhost",
		"7b3bf470-9456-11e8-9eb6-529269fb1459", "0313bafe-9457-11e8-9eb6-529269fb1459",
		[]string{"2f64e538-9457-11e8-9eb6-529269fb1459"}, nil,
		[]senders.SpanTag{
			{Key: "application", Value: "Wavefront"},
			{Key: "http.method", Value: "GET"},
		},
		nil); err != nil {
		t.Error("Failed SendSpan", err)
	}

	wf.Flush()
	wf.Close()
	assert.Equal(t, int64(0), wf.GetFailureCount(), "GetFailureCount")
}
