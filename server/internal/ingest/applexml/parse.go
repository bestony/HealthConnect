// Package applexml streams an Apple Health export.xml into domain facts.
package applexml

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"

	"com.bestony.health-connect/internal/domain"
)

const CodeXMLInvalid = "xml_invalid"

// Error is a parser failure with a stable code. Msg does not include health values.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return e.Code
	}
	return e.Code + ": " + e.Msg
}

// Handler receives facts in document order. One record is in memory at a time.
type Handler interface {
	OnExportDate(t time.Time) error
	OnCharacteristics(attrs map[string]string) error
	OnQuantity(sample domain.QuantitySample) error
	OnCategory(sample domain.CategorySample) error
	OnCorrelation(item domain.Correlation) error
	OnWorkout(item domain.Workout) error
	OnActivity(item domain.ActivitySummary) error
	OnIgnored(name string) error
}

// Parse reads export.xml. It does not build a document tree.
func Parse(r io.Reader, h Handler) error {
	dec := xml.NewDecoder(stripBOM(r))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return &Error{Code: CodeXMLInvalid, Msg: "token"}
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if err := dispatch(dec, start, h); err != nil {
			return err
		}
	}
}

func dispatch(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	switch start.Name.Local {
	case "HealthData":
		return nil
	case "ExportDate":
		return parseExportDate(dec, start, h)
	case "Me":
		return parseMe(dec, start, h)
	case "Record":
		return parseTopRecord(dec, start, h)
	case "Correlation":
		return parseCorrelation(dec, start, h)
	case "Workout":
		return parseWorkout(dec, start, h)
	case "ActivitySummary":
		return parseActivity(dec, start, h)
	default:
		if err := dec.Skip(); err != nil {
			return &Error{Code: CodeXMLInvalid, Msg: "skip"}
		}
		return h.OnIgnored(start.Name.Local)
	}
}

func parseExportDate(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	if err := dec.Skip(); err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "ExportDate"}
	}
	raw := attr(start, "value")
	t, _, err := parseAppleTime(raw)
	if err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "ExportDate"}
	}
	return h.OnExportDate(t)
}

func parseMe(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	if err := dec.Skip(); err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Me"}
	}
	return h.OnCharacteristics(attrMap(start))
}

func parseTopRecord(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	rec, err := decodeRecord(dec, start)
	if err != nil {
		return err
	}
	return emitRecord(rec, h)
}

func emitRecord(rec xmlRecord, h Handler) error {
	if isCategory(rec.Type, rec.Value) {
		sample, err := rec.category()
		if err != nil {
			return err
		}
		return h.OnCategory(sample)
	}
	sample, err := rec.quantity()
	if err != nil {
		return err
	}
	return h.OnQuantity(sample)
}

func parseCorrelation(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	var body struct {
		Metadata []xmlMeta   `xml:"MetadataEntry"`
		Records  []xmlRecord `xml:"Record"`
	}
	if err := dec.DecodeElement(&body, &start); err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Correlation"}
	}
	attrs := attrMap(start)
	item := domain.Correlation{
		Type:          attrs["type"],
		SourceName:    attrs["sourceName"],
		SourceVersion: attrs["sourceVersion"],
		Device:        attrs["device"],
		StartRaw:      attrs["startDate"],
		EndRaw:        attrs["endDate"],
		Metadata:      metaMap(body.Metadata),
		Attrs:         unknownAttrs(attrs, "type", "sourceName", "sourceVersion", "device", "creationDate", "startDate", "endDate"),
	}
	var err error
	item.ID, err = domain.NewID()
	if err != nil {
		return err
	}
	item.Start, item.TZOffsetMin, err = parseAppleTime(item.StartRaw)
	if err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Correlation.startDate"}
	}
	item.End, _, err = parseAppleTime(item.EndRaw)
	if err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Correlation.endDate"}
	}
	if raw := attrs["creationDate"]; raw != "" {
		created, _, err := parseAppleTime(raw)
		if err != nil {
			return &Error{Code: CodeXMLInvalid, Msg: "Correlation.creationDate"}
		}
		item.Creation = &created
	}
	for _, rec := range body.Records {
		if isCategory(rec.Type, rec.Value) {
			sample, err := rec.category()
			if err != nil {
				return err
			}
			item.Categories = append(item.Categories, sample)
			continue
		}
		sample, err := rec.quantity()
		if err != nil {
			return err
		}
		item.Quantities = append(item.Quantities, sample)
	}
	return h.OnCorrelation(item)
}

func parseWorkout(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	var body struct {
		Metadata []xmlMeta    `xml:"MetadataEntry"`
		Events   []xmlEvent   `xml:"WorkoutEvent"`
		Stats    []xmlStat    `xml:"WorkoutStatistics"`
		Routes   []xmlIgnored `xml:"WorkoutRoute"`
	}
	if err := dec.DecodeElement(&body, &start); err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Workout"}
	}
	attrs := attrMap(start)
	item := domain.Workout{
		ActivityType:  attrs["workoutActivityType"],
		SourceName:    attrs["sourceName"],
		SourceVersion: attrs["sourceVersion"],
		Device:        attrs["device"],
		StartRaw:      attrs["startDate"],
		EndRaw:        attrs["endDate"],
		DistanceUnit:  attrs["totalDistanceUnit"],
		EnergyUnit:    attrs["totalEnergyBurnedUnit"],
		Metadata:      metaMap(body.Metadata),
		Attrs: unknownAttrs(attrs,
			"workoutActivityType", "duration", "durationUnit",
			"totalDistance", "totalDistanceUnit", "totalEnergyBurned", "totalEnergyBurnedUnit",
			"sourceName", "sourceVersion", "device", "creationDate", "startDate", "endDate",
		),
	}
	var err error
	item.ID, err = domain.NewID()
	if err != nil {
		return err
	}
	item.Start, item.TZOffsetMin, err = parseAppleTime(item.StartRaw)
	if err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Workout.startDate"}
	}
	item.End, _, err = parseAppleTime(item.EndRaw)
	if err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "Workout.endDate"}
	}
	if raw := attrs["creationDate"]; raw != "" {
		created, _, err := parseAppleTime(raw)
		if err != nil {
			return &Error{Code: CodeXMLInvalid, Msg: "Workout.creationDate"}
		}
		item.Creation = &created
	}
	if raw := attrs["duration"]; raw != "" {
		sec, err := durationSeconds(raw, attrs["durationUnit"])
		if err != nil {
			return err
		}
		item.DurationSec = &sec
	}
	if raw := attrs["totalDistance"]; raw != "" {
		v, err := parseNumber(raw, "Workout.totalDistance")
		if err != nil {
			return err
		}
		item.Distance = &v
	}
	if raw := attrs["totalEnergyBurned"]; raw != "" {
		v, err := parseNumber(raw, "Workout.totalEnergyBurned")
		if err != nil {
			return err
		}
		item.Energy = &v
	}
	for _, ev := range body.Events {
		at, offset, err := parseAppleTime(ev.Date)
		if err != nil {
			return &Error{Code: CodeXMLInvalid, Msg: "WorkoutEvent.date"}
		}
		event := domain.WorkoutEvent{Type: ev.Type, At: at, AtRaw: ev.Date, TZOffsetMin: offset}
		if ev.Duration != "" {
			sec, err := durationSeconds(ev.Duration, ev.DurationUnit)
			if err != nil {
				return err
			}
			event.DurationSec = &sec
		}
		item.Events = append(item.Events, event)
	}
	for _, st := range body.Stats {
		stat := domain.WorkoutStatistic{Type: st.Type, Unit: st.Unit}
		if stat.Sum, err = optionalNumber(st.Sum, "WorkoutStatistics.sum"); err != nil {
			return err
		}
		if stat.Minimum, err = optionalNumber(st.Minimum, "WorkoutStatistics.minimum"); err != nil {
			return err
		}
		if stat.Maximum, err = optionalNumber(st.Maximum, "WorkoutStatistics.maximum"); err != nil {
			return err
		}
		if stat.Average, err = optionalNumber(st.Average, "WorkoutStatistics.average"); err != nil {
			return err
		}
		item.Statistics = append(item.Statistics, stat)
	}
	if err := h.OnWorkout(item); err != nil {
		return err
	}
	for range body.Routes {
		if err := h.OnIgnored("WorkoutRoute"); err != nil {
			return err
		}
	}
	return nil
}

func parseActivity(dec *xml.Decoder, start xml.StartElement, h Handler) error {
	if err := dec.Skip(); err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "ActivitySummary"}
	}
	attrs := attrMap(start)
	day, err := time.Parse("2006-01-02", attrs["dateComponents"])
	if err != nil {
		return &Error{Code: CodeXMLInvalid, Msg: "ActivitySummary.dateComponents"}
	}
	item := domain.ActivitySummary{
		Day:              day,
		ActiveEnergyUnit: attrs["activeEnergyBurnedUnit"],
		Attrs: unknownAttrs(attrs,
			"dateComponents", "activeEnergyBurned", "activeEnergyBurnedGoal", "activeEnergyBurnedUnit",
			"appleExerciseTime", "appleExerciseTimeGoal", "appleStandHours", "appleStandHoursGoal",
			"appleMoveTime", "appleMoveTimeGoal",
		),
	}
	if item.ActiveEnergy, err = optionalNumber(attrs["activeEnergyBurned"], "ActivitySummary.activeEnergyBurned"); err != nil {
		return err
	}
	if item.ActiveEnergyGoal, err = optionalNumber(attrs["activeEnergyBurnedGoal"], "ActivitySummary.activeEnergyBurnedGoal"); err != nil {
		return err
	}
	if item.ExerciseMinutes, err = optionalNumber(attrs["appleExerciseTime"], "ActivitySummary.appleExerciseTime"); err != nil {
		return err
	}
	if item.ExerciseGoal, err = optionalNumber(attrs["appleExerciseTimeGoal"], "ActivitySummary.appleExerciseTimeGoal"); err != nil {
		return err
	}
	if item.StandHours, err = optionalNumber(attrs["appleStandHours"], "ActivitySummary.appleStandHours"); err != nil {
		return err
	}
	if item.StandGoal, err = optionalNumber(attrs["appleStandHoursGoal"], "ActivitySummary.appleStandHoursGoal"); err != nil {
		return err
	}
	if item.MoveMinutes, err = optionalNumber(attrs["appleMoveTime"], "ActivitySummary.appleMoveTime"); err != nil {
		return err
	}
	if item.MoveGoal, err = optionalNumber(attrs["appleMoveTimeGoal"], "ActivitySummary.appleMoveTimeGoal"); err != nil {
		return err
	}
	return h.OnActivity(item)
}

// isCategory is true for HealthKit category identifiers even when the value is numeric.
// A numeric-looking stand-hour value must stay a category. Other non-numeric values do too.
func isCategory(typeID, value string) bool {
	if strings.Contains(typeID, "CategoryTypeIdentifier") {
		return true
	}
	if value == "" {
		return true
	}
	_, err := strconv.ParseFloat(value, 64)
	return err != nil
}

type xmlRecord struct {
	Type          string    `xml:"type,attr"`
	Unit          string    `xml:"unit,attr"`
	Value         string    `xml:"value,attr"`
	SourceName    string    `xml:"sourceName,attr"`
	SourceVersion string    `xml:"sourceVersion,attr"`
	Device        string    `xml:"device,attr"`
	CreationDate  string    `xml:"creationDate,attr"`
	StartDate     string    `xml:"startDate,attr"`
	EndDate       string    `xml:"endDate,attr"`
	Metadata      []xmlMeta `xml:"MetadataEntry"`
	extra         []xml.Attr
}

type xmlMeta struct {
	Key   string `xml:"key,attr"`
	Value string `xml:"value,attr"`
}

type xmlEvent struct {
	Type         string `xml:"type,attr"`
	Date         string `xml:"date,attr"`
	Duration     string `xml:"duration,attr"`
	DurationUnit string `xml:"durationUnit,attr"`
}

type xmlStat struct {
	Type    string `xml:"type,attr"`
	Sum     string `xml:"sum,attr"`
	Minimum string `xml:"minimum,attr"`
	Maximum string `xml:"maximum,attr"`
	Average string `xml:"average,attr"`
	Unit    string `xml:"unit,attr"`
}

type xmlIgnored struct{}

func decodeRecord(dec *xml.Decoder, start xml.StartElement) (xmlRecord, error) {
	var rec xmlRecord
	if err := dec.DecodeElement(&rec, &start); err != nil {
		return xmlRecord{}, &Error{Code: CodeXMLInvalid, Msg: "Record"}
	}
	return rec, nil
}

// UnmarshalXML keeps attributes that are not mapped to columns.
func (rec *xmlRecord) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	type raw xmlRecord
	var shadow raw
	if err := dec.DecodeElement(&shadow, &start); err != nil {
		return err
	}
	*rec = xmlRecord(shadow)
	rec.extra = start.Attr
	return nil
}

func (rec xmlRecord) quantity() (domain.QuantitySample, error) {
	start, offset, err := parseAppleTime(rec.StartDate)
	if err != nil {
		return domain.QuantitySample{}, &Error{Code: CodeXMLInvalid, Msg: "Record.startDate"}
	}
	end, _, err := parseAppleTime(rec.EndDate)
	if err != nil {
		return domain.QuantitySample{}, &Error{Code: CodeXMLInvalid, Msg: "Record.endDate"}
	}
	value, err := parseNumber(rec.Value, "Record.value")
	if err != nil {
		return domain.QuantitySample{}, err
	}
	sample := domain.QuantitySample{
		Type: rec.Type, Value: value, ValueText: rec.Value, Unit: rec.Unit,
		SourceName: rec.SourceName, SourceVersion: rec.SourceVersion, Device: rec.Device,
		Start: start, End: end, StartRaw: rec.StartDate, EndRaw: rec.EndDate,
		TZOffsetMin: offset, Metadata: metaMap(rec.Metadata), Attrs: recordAttrs(rec.extra),
	}
	if rec.CreationDate != "" {
		created, _, err := parseAppleTime(rec.CreationDate)
		if err != nil {
			return domain.QuantitySample{}, &Error{Code: CodeXMLInvalid, Msg: "Record.creationDate"}
		}
		sample.Creation = &created
	}
	return sample, nil
}

func (rec xmlRecord) category() (domain.CategorySample, error) {
	start, offset, err := parseAppleTime(rec.StartDate)
	if err != nil {
		return domain.CategorySample{}, &Error{Code: CodeXMLInvalid, Msg: "Record.startDate"}
	}
	end, _, err := parseAppleTime(rec.EndDate)
	if err != nil {
		return domain.CategorySample{}, &Error{Code: CodeXMLInvalid, Msg: "Record.endDate"}
	}
	sample := domain.CategorySample{
		Type: rec.Type, ValueText: rec.Value,
		SourceName: rec.SourceName, SourceVersion: rec.SourceVersion, Device: rec.Device,
		Start: start, End: end, StartRaw: rec.StartDate, EndRaw: rec.EndDate,
		TZOffsetMin: offset, Metadata: metaMap(rec.Metadata), Attrs: recordAttrs(rec.extra),
	}
	if rec.Unit != "" {
		if sample.Attrs == nil {
			sample.Attrs = map[string]string{}
		}
		sample.Attrs["unit"] = rec.Unit
	}
	if rec.CreationDate != "" {
		created, _, err := parseAppleTime(rec.CreationDate)
		if err != nil {
			return domain.CategorySample{}, &Error{Code: CodeXMLInvalid, Msg: "Record.creationDate"}
		}
		sample.Creation = &created
	}
	return sample, nil
}

func recordAttrs(attrs []xml.Attr) map[string]string {
	known := map[string]struct{}{
		"type": {}, "unit": {}, "value": {}, "sourceName": {}, "sourceVersion": {},
		"device": {}, "creationDate": {}, "startDate": {}, "endDate": {},
	}
	out := map[string]string{}
	for _, a := range attrs {
		if _, ok := known[a.Name.Local]; ok {
			continue
		}
		out[a.Name.Local] = a.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func attr(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func attrMap(start xml.StartElement) map[string]string {
	out := make(map[string]string, len(start.Attr))
	for _, a := range start.Attr {
		out[a.Name.Local] = a.Value
	}
	return out
}

func unknownAttrs(attrs map[string]string, known ...string) map[string]string {
	skip := make(map[string]struct{}, len(known))
	for _, name := range known {
		skip[name] = struct{}{}
	}
	out := map[string]string{}
	for key, value := range attrs {
		if _, ok := skip[key]; ok {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func metaMap(entries []xmlMeta) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		out[entry.Key] = entry.Value
	}
	return out
}

func parseAppleTime(raw string) (time.Time, int, error) {
	t, err := time.Parse("2006-01-02 15:04:05 -0700", raw)
	if err != nil {
		return time.Time{}, 0, err
	}
	_, offset := t.Zone()
	return t, offset / 60, nil
}

func parseNumber(raw, field string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, &Error{Code: CodeXMLInvalid, Msg: field}
	}
	return v, nil
}

func optionalNumber(raw, field string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	v, err := parseNumber(raw, field)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func durationSeconds(raw, unit string) (float64, error) {
	v, err := parseNumber(raw, "duration")
	if err != nil {
		return 0, err
	}
	switch strings.ToLower(unit) {
	case "s", "sec", "secs", "second", "seconds":
		return v, nil
	case "min", "mins", "minute", "minutes":
		return v * 60, nil
	case "h", "hr", "hrs", "hour", "hours":
		return v * 3600, nil
	default:
		return 0, &Error{Code: CodeXMLInvalid, Msg: "durationUnit"}
	}
}

func stripBOM(r io.Reader) io.Reader {
	br := bufio.NewReader(r)
	peek, err := br.Peek(3)
	if err == nil && bytes.Equal(peek, []byte{0xEF, 0xBB, 0xBF}) {
		_, _ = br.Discard(3)
	}
	return br
}
