// Package statslog emits the periodic DB-pool and Go-runtime stats log lines
// and builds their structured attributes. The monitoring Terraform defines
// log-based metrics that extract these fields into time series (#1613), so
// the message strings, field names, and emission intervals here are a
// contract with terraform/modules/monitoring/log_metrics.tf.
package statslog
