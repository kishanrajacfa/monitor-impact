package examples

import "log/slog"

// These examples are fixtures for reviewing log changes, not production handlers.
func recordBrew() {
	slog.Info("Your coffee is ready", "service", "coffee-machine")
}
