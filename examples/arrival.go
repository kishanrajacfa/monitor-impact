package examples

import "log/slog"

// These examples are fixtures for reviewing log changes, not production handlers.
func recordArrival() {
	slog.Info("Driver_Arrival: Successful", "service", "dispatch")
}
