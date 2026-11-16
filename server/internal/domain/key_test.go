package domain

import (
	"bytes"
	"testing"
)

func TestQuantityKeyDependsOnUserAndValue(t *testing.T) {
	sample := QuantitySample{
		Type: "HKQuantityTypeIdentifierStepCount", StartRaw: "a", EndRaw: "b",
		ValueText: "1", Unit: "count", SourceName: "iPhone",
	}
	a := QuantityKey("user-a", sample)
	b := QuantityKey("user-b", sample)
	if bytes.Equal(a, b) {
		t.Fatal("user id did not change the key")
	}
	sample.ValueText = "2"
	c := QuantityKey("user-a", sample)
	if bytes.Equal(a, c) {
		t.Fatal("value did not change the key")
	}
	if len(a) != 32 {
		t.Fatalf("len = %d", len(a))
	}
}
