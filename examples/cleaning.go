package examples

import "log/slog"

func recordCleaningFailure() {
	slog.Warn("Cleaning cycle failed", "service", "coffee-machine")
}
