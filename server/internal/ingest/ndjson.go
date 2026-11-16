package ingest

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"com.bestony.health-connect/internal/domain"
	"com.bestony.health-connect/internal/store/duckdb"
)

// LoadNDJSON reads stored batch files into the fact tables.
// importID is a dashed UUID. chars receives characteristic values.
func LoadNDJSON(ctx context.Context, facts *duckdb.DB, userID, importID string, paths []string) (map[string]string, error) {
	chars := map[string]string{}
	err := facts.WithLoader(ctx, func(loader *duckdb.Loader) error {
		for _, path := range paths {
			if err := loadFile(loader, userID, importID, path, chars); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		_ = facts.DeleteStaging(ctx, importID)
		return nil, err
	}
	if _, err := facts.CommitImport(ctx, userID, importID, "replace"); err != nil {
		_ = facts.DeleteStaging(ctx, importID)
		return nil, err
	}
	return chars, nil
}

func loadFile(loader *duckdb.Loader, userID, importID, path string, chars map[string]string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 32<<20)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		rec, err := decodeRecord([]byte(line))
		if err != nil {
			return &Error{Code: "ndjson_invalid", Msg: "line " + strconv.Itoa(lineNo)}
		}
		if err := applyRecord(loader, userID, importID, rec, chars); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

type ndRecord struct {
	kind     string
	typeID   string
	uuid     string
	start    string
	end      string
	source   string
	device   string
	metadata map[string]any
	fields   map[string]any
	objects  []ndRecord
	payload  json.RawMessage
}

func decodeRecord(raw []byte) (ndRecord, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return ndRecord{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return ndRecord{}, fmt.Errorf("extra json")
	}
	return recordFromMap(obj, raw)
}

func recordFromMap(obj map[string]any, payload json.RawMessage) (ndRecord, error) {
	rec := ndRecord{
		kind: asString(obj["kind"]), typeID: asString(obj["typeIdentifier"]),
		uuid: asString(obj["uuid"]), start: asString(obj["start"]), end: asString(obj["end"]),
		source: asString(obj["source"]), device: asString(obj["device"]),
		payload: payload,
	}
	if meta, ok := obj["metadata"].(map[string]any); ok {
		rec.metadata = meta
	}
	if rawObjects, ok := obj["objects"].([]any); ok {
		for _, item := range rawObjects {
			childMap, ok := item.(map[string]any)
			if !ok {
				return ndRecord{}, fmt.Errorf("object")
			}
			body, err := json.Marshal(childMap)
			if err != nil {
				return ndRecord{}, err
			}
			child, err := recordFromMap(childMap, body)
			if err != nil {
				return ndRecord{}, err
			}
			rec.objects = append(rec.objects, child)
		}
	}
	rec.fields = map[string]any{}
	for key, value := range obj {
		switch key {
		case "kind", "typeIdentifier", "uuid", "start", "end", "source", "device", "metadata", "objects":
			continue
		default:
			rec.fields[key] = value
		}
	}
	return rec, nil
}

func applyRecord(loader *duckdb.Loader, userID, importID string, rec ndRecord, chars map[string]string) error {
	if err := loader.AppendHealthRecord(userID, importID, healthRow(rec)); err != nil {
		return err
	}
	switch rec.kind {
	case "characteristic":
		if rec.typeID != "" {
			chars[rec.typeID] = characteristicValue(rec.fields)
		}
		return nil
	case "activitySummary":
		return appendActivity(loader, userID, importID, rec)
	case "sample":
		if len(rec.objects) > 0 {
			return appendCorrelation(loader, userID, importID, rec)
		}
		if strings.Contains(rec.typeID, "Workout") || rec.fields["activityType"] != nil {
			return appendWorkout(loader, userID, importID, rec)
		}
		if value, unit, ok := quantityValue(rec.fields); ok {
			return appendQuantity(loader, userID, importID, rec, value, unit)
		}
		if text, ok := scalarText(rec.fields["value"]); ok {
			return appendCategory(loader, userID, importID, rec, text)
		}
	}
	return nil
}

func healthRow(rec ndRecord) duckdb.HealthRecord {
	row := duckdb.HealthRecord{
		Kind: rec.kind, TypeIdentifier: rec.typeID, UUID: rec.uuid,
		Source: rec.source, Device: rec.device, Payload: rec.payload,
	}
	if rec.start != "" {
		if t, offset, err := parseStamp(rec.start); err == nil {
			row.Start = &t
			row.TZOffsetMin = &offset
		}
	}
	if rec.end != "" {
		if t, _, err := parseStamp(rec.end); err == nil {
			row.End = &t
		}
	}
	return row
}

func appendQuantity(loader *duckdb.Loader, userID, importID string, rec ndRecord, value float64, unit string) error {
	sample, err := timedSample(rec)
	if err != nil {
		return err
	}
	return loader.AppendQuantity(userID, importID, domain.QuantitySample{
		Type: rec.typeID, Value: value, ValueText: scalarString(rec.fields["value"]), Unit: unit,
		SourceName: rec.source, Device: rec.device,
		Start: sample.start, End: sample.end, StartRaw: rec.start, EndRaw: rec.end,
		TZOffsetMin: sample.offset, Metadata: stringMap(rec.metadata),
	})
}

func appendCategory(loader *duckdb.Loader, userID, importID string, rec ndRecord, text string) error {
	sample, err := timedSample(rec)
	if err != nil {
		return err
	}
	return loader.AppendCategory(userID, importID, domain.CategorySample{
		Type: rec.typeID, ValueText: text, SourceName: rec.source, Device: rec.device,
		Start: sample.start, End: sample.end, StartRaw: rec.start, EndRaw: rec.end,
		TZOffsetMin: sample.offset, Metadata: stringMap(rec.metadata),
	})
}

func appendCorrelation(loader *duckdb.Loader, userID, importID string, rec ndRecord) error {
	sample, err := timedSample(rec)
	if err != nil {
		return err
	}
	id := rec.uuid
	if id == "" {
		id, err = domain.NewID()
		if err != nil {
			return err
		}
	}
	item := domain.Correlation{
		ID: id, Type: rec.typeID, SourceName: rec.source, Device: rec.device,
		Start: sample.start, End: sample.end, StartRaw: rec.start, EndRaw: rec.end,
		TZOffsetMin: sample.offset, Metadata: stringMap(rec.metadata),
	}
	for _, child := range rec.objects {
		if err := loader.AppendHealthRecord(userID, importID, healthRow(child)); err != nil {
			return err
		}
		if value, unit, ok := quantityValue(child.fields); ok {
			childSample, err := timedSample(child)
			if err != nil {
				return err
			}
			item.Quantities = append(item.Quantities, domain.QuantitySample{
				Type: child.typeID, Value: value, ValueText: scalarString(child.fields["value"]), Unit: unit,
				SourceName: child.source, Device: child.device,
				Start: childSample.start, End: childSample.end, StartRaw: child.start, EndRaw: child.end,
				TZOffsetMin: childSample.offset,
			})
			continue
		}
		if text, ok := scalarText(child.fields["value"]); ok {
			childSample, err := timedSample(child)
			if err != nil {
				return err
			}
			item.Categories = append(item.Categories, domain.CategorySample{
				Type: child.typeID, ValueText: text, SourceName: child.source, Device: child.device,
				Start: childSample.start, End: childSample.end, StartRaw: child.start, EndRaw: child.end,
				TZOffsetMin: childSample.offset,
			})
		}
	}
	return loader.AppendCorrelation(userID, importID, item)
}

func appendWorkout(loader *duckdb.Loader, userID, importID string, rec ndRecord) error {
	sample, err := timedSample(rec)
	if err != nil {
		return err
	}
	id := rec.uuid
	if id == "" {
		id, err = domain.NewID()
		if err != nil {
			return err
		}
	}
	item := domain.Workout{
		ID: id, ActivityType: scalarString(rec.fields["activityType"]), SourceName: rec.source, Device: rec.device,
		Start: sample.start, End: sample.end, StartRaw: rec.start, EndRaw: rec.end, TZOffsetMin: sample.offset,
		Metadata: stringMap(rec.metadata),
	}
	if text := scalarString(rec.fields["duration"]); text != "" {
		if v, err := strconv.ParseFloat(text, 64); err == nil {
			item.DurationSec = &v
		}
	}
	if stats, ok := rec.fields["statistics"].([]any); ok {
		for _, raw := range stats {
			obj, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			stat := domain.WorkoutStatistic{Type: asString(obj["type"]), Unit: asString(obj["unit"])}
			stat.Sum = floatPtr(obj["sum"])
			stat.Average = floatPtr(obj["average"])
			stat.Minimum = floatPtr(obj["minimum"])
			stat.Maximum = floatPtr(obj["maximum"])
			item.Statistics = append(item.Statistics, stat)
		}
	}
	return loader.AppendWorkout(userID, importID, item)
}

func appendActivity(loader *duckdb.Loader, userID, importID string, rec ndRecord) error {
	year, yok := asInt(rec.fields["year"])
	month, mok := asInt(rec.fields["month"])
	day, dok := asInt(rec.fields["day"])
	if !yok || !mok || !dok {
		return &Error{Code: "ndjson_invalid", Msg: "activitySummary"}
	}
	item := domain.ActivitySummary{
		Day:              time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC),
		ActiveEnergyUnit: "kcal",
		ActiveEnergy:     floatPtr(rec.fields["activeEnergyBurnedKcal"]),
		ActiveEnergyGoal: floatPtr(rec.fields["activeEnergyBurnedGoalKcal"]),
		ExerciseMinutes:  floatPtr(rec.fields["appleExerciseTimeMin"]),
		ExerciseGoal:     floatPtr(rec.fields["exerciseTimeGoalMin"]),
		StandHours:       floatPtr(rec.fields["appleStandHours"]),
		StandGoal:        floatPtr(rec.fields["standHoursGoal"]),
		MoveMinutes:      floatPtr(rec.fields["appleMoveTimeMin"]),
		MoveGoal:         floatPtr(rec.fields["appleMoveTimeGoalMin"]),
	}
	return loader.AppendActivity(userID, importID, item)
}

type stamp struct {
	start  time.Time
	end    time.Time
	offset int
}

func timedSample(rec ndRecord) (stamp, error) {
	start, offset, err := parseStamp(rec.start)
	if err != nil {
		return stamp{}, &Error{Code: "ndjson_invalid", Msg: "start"}
	}
	end, _, err := parseStamp(rec.end)
	if err != nil {
		return stamp{}, &Error{Code: "ndjson_invalid", Msg: "end"}
	}
	return stamp{start: start, end: end, offset: offset}, nil
}

func parseStamp(raw string) (time.Time, int, error) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, raw); err == nil {
			_, offset := t.Zone()
			return t, offset / 60, nil
		}
	}
	return time.Time{}, 0, fmt.Errorf("time")
}

func characteristicValue(fields map[string]any) string {
	year, yok := asInt(fields["year"])
	month, mok := asInt(fields["month"])
	day, dok := asInt(fields["day"])
	if yok && mok && dok && len(fields) == 3 {
		return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
	}
	if len(fields) == 1 {
		if text, ok := scalarText(fields["value"]); ok {
			return text
		}
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return ""
	}
	return string(body)
}

func quantityValue(fields map[string]any) (float64, string, bool) {
	unit, ok := fields["unit"].(string)
	if !ok || unit == "" {
		return 0, "", false
	}
	value := floatPtr(fields["value"])
	if value == nil {
		return 0, "", false
	}
	return *value, unit, true
}

func scalarText(v any) (string, bool) {
	switch t := v.(type) {
	case nil:
		return "", false
	case string:
		return t, true
	case json.Number:
		return t.String(), true
	case bool:
		if t {
			return "true", true
		}
		return "false", true
	default:
		return "", false
	}
}

func scalarString(v any) string {
	text, _ := scalarText(v)
	return text
}

func floatPtr(v any) *float64 {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return nil
		}
		return &f
	case float64:
		return &t
	default:
		return nil
	}
}

func asInt(v any) (int, bool) {
	switch t := v.(type) {
	case json.Number:
		n, err := t.Int64()
		if err != nil {
			f, ferr := t.Float64()
			if ferr != nil {
				return 0, false
			}
			return int(f), true
		}
		return int(n), true
	case float64:
		return int(t), true
	default:
		return 0, false
	}
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func stringMap(in map[string]any) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		if text, ok := scalarText(value); ok {
			out[key] = text
			continue
		}
		body, err := json.Marshal(value)
		if err != nil {
			continue
		}
		out[key] = string(body)
	}
	return out
}
