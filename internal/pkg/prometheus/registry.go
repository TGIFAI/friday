package prometheus

import "github.com/prometheus/client_golang/prometheus"

var (
	registry = prometheus.NewRegistry()
)

func GetRegistry() *prometheus.Registry {
	return registry
}
