package applexml

import (
	"os"
	"strings"
	"testing"
	"time"

	"com.bestony.health-connect/internal/domain"
)

type collect struct {
	export       time.Time
	me           map[string]string
	quantities   []domain.QuantitySample
	categories   []domain.CategorySample
	correlations []domain.Correlation
	workouts     []domain.Workout
	activities   []domain.ActivitySummary
	ignored      []string
}

func (c *collect) OnExportDate(t time.Time) error { c.export = t; return nil }
func (c *collect) OnCharacteristics(attrs map[string]string) error {
	c.me = attrs
	return nil
}
func (c *collect) OnQuantity(sample domain.QuantitySample) error {
	c.quantities = append(c.quantities, sample)
	return nil
}
func (c *collect) OnCategory(sample domain.CategorySample) error {
	c.categories = append(c.categories, sample)
	return nil
}
func (c *collect) OnCorrelation(item domain.Correlation) error {
	c.correlations = append(c.correlations, item)
	return nil
}
func (c *collect) OnWorkout(item domain.Workout) error {
	c.workouts = append(c.workouts, item)
	return nil
}
func (c *collect) OnActivity(item domain.ActivitySummary) error {
	c.activities = append(c.activities, item)
	return nil
}
func (c *collect) OnIgnored(name string) error {
	c.ignored = append(c.ignored, name)
	return nil
}

func TestParseFixture(t *testing.T) {
	f, err := os.Open("../../../testdata/export_min.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var got collect
	if err := Parse(f, &got); err != nil {
		t.Fatal(err)
	}
	if got.export.UTC().Format(time.RFC3339) != "2024-01-02T00:00:00Z" {
		t.Fatalf("export = %s", got.export)
	}
	if got.me["HKCharacteristicTypeIdentifierDateOfBirth"] != "1990-05-06" {
		t.Fatalf("me = %#v", got.me)
	}
	if len(got.quantities) != 1 || got.quantities[0].Value != 1200 || got.quantities[0].TZOffsetMin != 480 {
		t.Fatalf("quantities = %#v", got.quantities)
	}
	if got.quantities[0].Metadata["HKMetadataKeyWasUserEntered"] != "0" {
		t.Fatalf("metadata = %#v", got.quantities[0].Metadata)
	}
	if len(got.categories) != 2 {
		t.Fatalf("categories = %d", len(got.categories))
	}
	var stand bool
	for _, sample := range got.categories {
		if sample.Type == "HKCategoryTypeIdentifierAppleStandHour" {
			stand = sample.ValueText == "0"
		}
	}
	if !stand {
		t.Fatal("numeric category value was not kept as a category")
	}
	if len(got.correlations) != 1 || len(got.correlations[0].Quantities) != 2 {
		t.Fatalf("correlation = %#v", got.correlations)
	}
	if got.correlations[0].Quantities[0].Value != 118 || got.correlations[0].Quantities[1].Value != 76 {
		t.Fatalf("bp = %#v", got.correlations[0].Quantities)
	}
	if len(got.workouts) != 1 {
		t.Fatalf("workouts = %d", len(got.workouts))
	}
	w := got.workouts[0]
	if w.DurationSec == nil || *w.DurationSec != 1800 || w.Attrs["indoor"] != "0" {
		t.Fatalf("workout = %#v", w)
	}
	if len(w.Events) != 1 || w.Events[0].DurationSec == nil || *w.Events[0].DurationSec != 60 {
		t.Fatalf("events = %#v", w.Events)
	}
	if len(w.Statistics) != 1 || w.Statistics[0].Average == nil || *w.Statistics[0].Average != 150 {
		t.Fatalf("stats = %#v", w.Statistics)
	}
	if len(got.activities) != 1 || got.activities[0].Day.Format("2006-01-02") != "2024-01-02" {
		t.Fatalf("activity = %#v", got.activities)
	}
	if got.activities[0].MoveMinutes == nil || *got.activities[0].MoveMinutes != 20 {
		t.Fatal("move minutes missing")
	}
	if strings.Join(got.ignored, ",") != "WorkoutRoute,ClinicalRecord" {
		t.Fatalf("ignored = %#v", got.ignored)
	}
}

func TestParseRejectsBadTime(t *testing.T) {
	body := `<HealthData><Record type="HKQuantityTypeIdentifierStepCount" value="1" unit="count" startDate="bad" endDate="2024-01-02 08:00:00 +0800"/></HealthData>`
	err := Parse(strings.NewReader(body), &collect{})
	parsed, ok := err.(*Error)
	if !ok || parsed.Code != CodeXMLInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestParseNonNumericQuantityBecomesCategory(t *testing.T) {
	body := `<HealthData><Record type="HKQuantityTypeIdentifierHeartRate" value="high" unit="count/min" sourceName="Watch" startDate="2024-01-02 08:00:00 +0800" endDate="2024-01-02 08:00:01 +0800"/></HealthData>`
	var got collect
	if err := Parse(strings.NewReader(body), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.categories) != 1 || got.categories[0].ValueText != "high" || len(got.quantities) != 0 {
		t.Fatalf("categories = %#v quantities = %#v", got.categories, got.quantities)
	}
}

func TestParseStripsBOM(t *testing.T) {
	body := "\uFEFF<HealthData><ExportDate value=\"2024-01-02 08:00:00 +0800\"/></HealthData>"
	var got collect
	if err := Parse(strings.NewReader(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.export.IsZero() {
		t.Fatal("export date missing")
	}
}
