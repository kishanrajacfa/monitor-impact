package examples

import "log/slog"

func recordCleaningFailure() {
	slog.Error("Cleaning cycle failed", "service", "coffee-machine")
}
