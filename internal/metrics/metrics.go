package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WebhookHMACFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "paystable_webhook_hmac_failures_total",
		Help: "Total webhooks rejected due to HMAC mismatch",
	})
	VerificationMismatches = promauto.NewCounter(prometheus.CounterOpts{
		Name: "paystable_verification_mismatches_total",
		Help: "Total transactions where webhook signal disagreed with verified result",
	})
	OutboxDeliveryFailures = promauto.NewCounter(prometheus.CounterOpts{
		Name: "paystable_outbox_delivery_failures_total",
		Help: "Total outbox delivery exhaustions",
	})
	TxnIndeterminate = promauto.NewCounter(prometheus.CounterOpts{
		Name: "paystable_txn_indeterminate_total",
		Help: "Total transactions that reached INDETERMINATE state",
	})
	GatewayDegraded = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "paystable_gateway_degraded",
		Help: "1 if gateway is currently in degraded mode, 0 otherwise",
	}, []string{"gateway"})
	PollLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "paystable_poll_latency_seconds",
		Help:    "Latency of gateway verification poll calls",
		Buckets: prometheus.DefBuckets,
	}, []string{"gateway"})
)
