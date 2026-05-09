package pglog

import (
	"log/slog"
	"testing"
)

func TestIsReliabilityEvent(t *testing.T) {
	cases := []struct {
		message string
		want    bool
	}{
		{message: "litellm_retry", want: true},
		{message: "litellm_circuit_opened", want: true},
		{message: "ocr_retry", want: true},
		{message: "ocr_response_rejected", want: true},
		{message: "ocr_fallback_triggered", want: true},
		{message: "translation completed", want: false},
	}

	for _, tc := range cases {
		if got := isReliabilityEvent(tc.message); got != tc.want {
			t.Fatalf("isReliabilityEvent(%q) = %v, want %v", tc.message, got, tc.want)
		}
	}
}

func TestFieldString(t *testing.T) {
	fields := map[string]slog.Value{
		"plain": slog.StringValue("value"),
		"int":   slog.Int64Value(42),
	}

	if got := fieldString(fields, "plain"); got != "value" {
		t.Fatalf("expected plain string value, got %q", got)
	}
	if got := fieldString(fields, "int"); got != "42" {
		t.Fatalf("expected converted int value, got %q", got)
	}
	if got := fieldString(fields, "missing"); got != "" {
		t.Fatalf("expected empty string for missing field, got %q", got)
	}
}

func TestFieldInt64(t *testing.T) {
	tests := []struct {
		name   string
		value  slog.Value
		want   int64
		hasKey bool
	}{
		{name: "int64", value: slog.Int64Value(10), want: 10, hasKey: true},
		{name: "uint64", value: slog.Uint64Value(11), want: 11, hasKey: true},
		{name: "float64", value: slog.Float64Value(12.8), want: 12, hasKey: true},
		{name: "string int", value: slog.StringValue("13"), want: 13, hasKey: true},
		{name: "invalid string", value: slog.StringValue("nope"), want: 0, hasKey: true},
		{name: "missing", want: 0, hasKey: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := map[string]slog.Value{}
			if tc.hasKey {
				fields["k"] = tc.value
			}
			if got := fieldInt64(fields, "k"); got != tc.want {
				t.Fatalf("expected %d, got %d", tc.want, got)
			}
		})
	}
}
