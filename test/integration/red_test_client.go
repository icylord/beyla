//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/mariomac/guara/pkg/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/beyla/v2/test/integration/components/jaeger"
	"github.com/grafana/beyla/v2/test/integration/components/prom"
)

func testClientWithMethodAndStatusCode(t *testing.T, method string, statusCode int, traces bool) {
	// Eventually, Prometheus would make this query visible
	pq := prom.Client{HostPort: prometheusHostPort}
	var results []prom.Result
	test.Eventually(t, testTimeout, func(t require.TestingT) {
		var err error
		results, err = pq.Query(`http_client_request_duration_seconds_count{` +
			fmt.Sprintf(`http_request_method="%s",`, method) +
			fmt.Sprintf(`http_response_status_code="%d",`, statusCode) +
			`http_route="/oss/",` +
			`server_address="grafana.com",` +
			`service_namespace="integration-test",` +
			`service_name="pingclient"}`)
		require.NoError(t, err)
		enoughPromResults(t, results)
		val := totalPromCount(t, results)
		assert.LessOrEqual(t, 1, val)
	})

	test.Eventually(t, testTimeout, func(t require.TestingT) {
		var err error
		results, err = pq.Query(`http_client_request_body_size_bytes_count{` +
			fmt.Sprintf(`http_request_method="%s",`, method) +
			fmt.Sprintf(`http_response_status_code="%d",`, statusCode) +
			`http_route="/oss/",` +
			`server_address="grafana.com",` +
			`service_namespace="integration-test",` +
			`service_name="pingclient"}`)
		require.NoError(t, err)
		enoughPromResults(t, results)
		val := totalPromCount(t, results)
		assert.LessOrEqual(t, 1, val)
	})

	if !traces {
		return
	}

	var trace jaeger.Trace
	test.Eventually(t, testTimeout, func(t require.TestingT) {
		resp, err := http.Get(jaegerQueryURL + fmt.Sprintf("?service=pingclient&operation=%s", method))
		require.NoError(t, err)
		if resp == nil {
			return
		}
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var tq jaeger.TracesQuery
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&tq))
		traces := tq.FindBySpan(jaeger.Tag{Key: "http.response.status_code", Type: "int64", Value: float64(statusCode)})
		require.GreaterOrEqual(t, len(traces), 1)
		trace = traces[0]
	}, test.Interval(100*time.Millisecond))

	res := trace.FindByOperationName(method)
	require.Len(t, res, 1)
	parent := res[0]

	addr, ok := jaeger.FindIn(parent.Tags, "server.address")
	assert.True(t, ok)
	assert.Equal(t, "grafana.com", addr.Value)

	addr, ok = jaeger.FindIn(parent.Tags, "server.port")
	assert.True(t, ok)
	assert.Equal(t, float64(443), addr.Value)
}

func testREDMetricsForClientHTTPLibrary(t *testing.T) {
	testClientWithMethodAndStatusCode(t, "GET", 200, true)
	testClientWithMethodAndStatusCode(t, "OPTIONS", 204, true)
}

func testREDMetricsForClientHTTPLibraryNoTraces(t *testing.T) {
	testClientWithMethodAndStatusCode(t, "GET", 200, false)
	testClientWithMethodAndStatusCode(t, "OPTIONS", 204, false)
}
