package examples

import "log/slog"

func recordPaymentFailure() {
	slog.Error("Payment authorization failed", "service", "payments")
}
