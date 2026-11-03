package examples

import "log/slog"

func recordPaymentFailure() {
	slog.Warn("Payment authorization failed", "service", "payments")
}
