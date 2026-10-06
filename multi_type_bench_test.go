package csv

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Priority is a custom type implementing encoding.TextMarshaler and encoding.TextUnmarshaler.
type Priority uint8

const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
)

func (p Priority) MarshalText() ([]byte, error) {
	switch p {
	case PriorityLow:
		return []byte("LOW"), nil
	case PriorityMedium:
		return []byte("MEDIUM"), nil
	case PriorityHigh:
		return []byte("HIGH"), nil
	default:
		return []byte("UNKNOWN"), nil
	}
}

func (p *Priority) UnmarshalText(text []byte) error {
	switch string(text) {
	case "LOW":
		*p = PriorityLow
	case "MEDIUM":
		*p = PriorityMedium
	case "HIGH":
		*p = PriorityHigh
	default:
		return errors.New("invalid priority")
	}
	return nil
}

// MultiTypeStruct tests struct reflection path.
type MultiTypeStruct struct {
	ID          int64     `csv:"id"`
	StatusCode  uint32    `csv:"status_code"`
	LatencyMs   float64   `csv:"latency_ms"`
	Success     bool      `csv:"success"`
	Method      string    `csv:"method"`
	RequestPath string    `csv:"request_path"`
	Priority    Priority  `csv:"priority"`
	Timestamp   time.Time `csv:"timestamp"`
	UserAgent   string    `csv:"user_agent,omitempty"`
}

// MultiTypeGeneric tests zero-reflection generic path.
type MultiTypeGeneric struct {
	ID          int64
	StatusCode  uint32
	LatencyMs   float64
	Success     bool
	Method      string
	RequestPath string
	Priority    Priority
	Timestamp   time.Time
	UserAgent   string
}

func (g *MultiTypeGeneric) CSVHeader() []string {
	return []string{"id", "status_code", "latency_ms", "success", "method", "request_path", "priority", "timestamp", "user_agent"}
}

func (g *MultiTypeGeneric) MarshalCSVRecord(w *Writer) error {
	w.WriteFieldBytes(strconv.AppendInt(nil, g.ID, 10))
	w.WriteDelimiter()
	w.WriteFieldBytes(strconv.AppendUint(nil, uint64(g.StatusCode), 10))
	w.WriteDelimiter()
	w.WriteFieldBytes(strconv.AppendFloat(nil, g.LatencyMs, 'f', 2, 64))
	w.WriteDelimiter()
	if g.Success {
		w.WriteFieldBytes([]byte("true"))
	} else {
		w.WriteFieldBytes([]byte("false"))
	}
	w.WriteDelimiter()
	w.WriteFieldBytes([]byte(g.Method))
	w.WriteDelimiter()
	w.WriteFieldBytes([]byte(g.RequestPath))
	w.WriteDelimiter()
	text, _ := g.Priority.MarshalText()
	w.WriteFieldBytes(text)
	w.WriteDelimiter()
	w.WriteFieldBytes([]byte(g.Timestamp.Format(time.RFC3339)))
	w.WriteDelimiter()
	if g.UserAgent != "" {
		w.WriteFieldBytes([]byte(g.UserAgent))
	}
	return nil
}

func (g *MultiTypeGeneric) UnmarshalCSVRecord(rec *Record) error {
	id, err := rec.FieldInt64(0)
	if err != nil {
		return err
	}
	g.ID = id

	sc, err := rec.FieldUint(1)
	if err != nil {
		return err
	}
	g.StatusCode = uint32(sc)

	lat, err := rec.FieldFloat64(2)
	if err != nil {
		return err
	}
	g.LatencyMs = lat

	succ, err := rec.FieldBool(3)
	if err != nil {
		return err
	}
	g.Success = succ

	g.Method = rec.FieldString(4)
	g.RequestPath = rec.FieldString(5)

	if err := g.Priority.UnmarshalText(rec.Field(6)); err != nil {
		return err
	}

	tm, err := time.Parse(time.RFC3339, rec.FieldString(7))
	if err != nil {
		return err
	}
	g.Timestamp = tm

	if rec.NumFields() > 8 {
		g.UserAgent = rec.FieldString(8)
	}
	return nil
}

func makeMultiTypeDataset(n int) ([]MultiTypeStruct, []MultiTypeGeneric, []byte) {
	tm, _ := time.Parse(time.RFC3339, "2026-10-07T12:00:00Z")
	structs := make([]MultiTypeStruct, n)
	generics := make([]MultiTypeGeneric, n)

	methods := []string{"GET", "POST", "PUT", "DELETE"}
	priorities := []Priority{PriorityLow, PriorityMedium, PriorityHigh}

	var sb strings.Builder
	sb.Grow(n * 120)
	sb.WriteString("id,status_code,latency_ms,success,method,request_path,priority,timestamp,user_agent\n")

	for i := 0; i < n; i++ {
		method := methods[i%4]
		priority := priorities[i%3]
		ua := ""
		if i%2 == 0 {
			ua = "Mozilla/5.0 (X11; Linux x86_64) Antigravity/2.0"
		}
		path := "/api/v1/resource/" + strconv.Itoa(i)
		if i%5 == 0 {
			path = "/search?query=\"benchmark, test\"&limit=50"
		}

		s := MultiTypeStruct{
			ID:          int64(i + 1),
			StatusCode:  uint32(200 + (i % 5)),
			LatencyMs:   12.34 + float64(i)*0.01,
			Success:     i%20 != 0,
			Method:      method,
			RequestPath: path,
			Priority:    priority,
			Timestamp:   tm,
			UserAgent:   ua,
		}
		structs[i] = s

		generics[i] = MultiTypeGeneric{
			ID:          s.ID,
			StatusCode:  s.StatusCode,
			LatencyMs:   s.LatencyMs,
			Success:     s.Success,
			Method:      s.Method,
			RequestPath: s.RequestPath,
			Priority:    s.Priority,
			Timestamp:   s.Timestamp,
			UserAgent:   s.UserAgent,
		}

		// CSV line
		pText, _ := priority.MarshalText()
		qPath := path
		if strings.ContainsAny(path, `,"`) {
			qPath = `"` + strings.ReplaceAll(path, `"`, `""`) + `"`
		}
		qUa := ua
		if strings.ContainsAny(ua, `,"`) {
			qUa = `"` + strings.ReplaceAll(ua, `"`, `""`) + `"`
		}

		sb.WriteString(strconv.Itoa(i+1) + "," +
			strconv.Itoa(int(s.StatusCode)) + "," +
			strconv.FormatFloat(s.LatencyMs, 'f', 2, 64) + "," +
			strconv.FormatBool(s.Success) + "," +
			method + "," +
			qPath + "," +
			string(pText) + "," +
			tm.Format(time.RFC3339) + "," +
			qUa + "\n")
	}

	return structs, generics, []byte(sb.String())
}

func TestMultiTypeEquivalence(t *testing.T) {
	structs, generics, csvData := makeMultiTypeDataset(20)

	var unmarshaledStructs []MultiTypeStruct
	if err := Unmarshal(csvData, &unmarshaledStructs); err != nil {
		t.Fatalf("Unmarshal struct failed: %v", err)
	}

	var unmarshaledGenerics []MultiTypeGeneric
	if err := UnmarshalTo(csvData, &unmarshaledGenerics); err != nil {
		t.Fatalf("UnmarshalTo generic failed: %v", err)
	}

	if len(unmarshaledStructs) != 20 || len(unmarshaledGenerics) != 20 {
		t.Fatalf("row count mismatch: %d vs %d", len(unmarshaledStructs), len(unmarshaledGenerics))
	}

	for i := 0; i < 20; i++ {
		if unmarshaledStructs[i].ID != structs[i].ID ||
			unmarshaledStructs[i].StatusCode != structs[i].StatusCode ||
			unmarshaledStructs[i].Priority != structs[i].Priority ||
			unmarshaledStructs[i].RequestPath != structs[i].RequestPath {
			t.Fatalf("struct unmarshal mismatch at row %d:\nexp: %+v\ngot: %+v", i, structs[i], unmarshaledStructs[i])
		}

		if unmarshaledGenerics[i].ID != generics[i].ID ||
			unmarshaledGenerics[i].StatusCode != generics[i].StatusCode ||
			unmarshaledGenerics[i].Priority != generics[i].Priority ||
			unmarshaledGenerics[i].RequestPath != generics[i].RequestPath {
			t.Fatalf("generic unmarshal mismatch at row %d:\nexp: %+v\ngot: %+v", i, generics[i], unmarshaledGenerics[i])
		}
	}

	// Test Marshal
	mStruct, err := Marshal(structs)
	if err != nil {
		t.Fatalf("Marshal struct failed: %v", err)
	}

	mGen, err := MarshalSlice[MultiTypeGeneric, *MultiTypeGeneric](generics)
	if err != nil {
		t.Fatalf("MarshalSlice generic failed: %v", err)
	}

	if len(mStruct) == 0 || len(mGen) == 0 {
		t.Fatal("empty marshal output")
	}
}

// -------------------------------------------------------------
// Multi-Type Struct vs Generic UNMARSHAL Benchmarks (50,000 Rows)
// -------------------------------------------------------------

func BenchmarkMultiType_Unmarshal_Struct_Sequential(b *testing.B) {
	_, _, csvData := makeMultiTypeDataset(50000)
	b.SetBytes(int64(len(csvData)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var out []MultiTypeStruct
		if err := Unmarshal(csvData, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultiType_Unmarshal_Generic_Sequential(b *testing.B) {
	_, _, csvData := makeMultiTypeDataset(50000)
	b.SetBytes(int64(len(csvData)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var out []MultiTypeGeneric
		if err := UnmarshalTo(csvData, &out); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultiType_Unmarshal_Struct_Parallel_16W(b *testing.B) {
	_, _, csvData := makeMultiTypeDataset(50000)
	b.SetBytes(int64(len(csvData)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var out []MultiTypeStruct
		if err := ParallelUnmarshal(csvData, &out, ParallelOptions{Workers: 16}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultiType_Unmarshal_Generic_Parallel_16W(b *testing.B) {
	_, _, csvData := makeMultiTypeDataset(50000)
	b.SetBytes(int64(len(csvData)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		var out []MultiTypeGeneric
		if err := ParallelUnmarshalTo(csvData, &out, ParallelOptions{Workers: 16}); err != nil {
			b.Fatal(err)
		}
	}
}

// -------------------------------------------------------------
// Multi-Type Struct vs Generic MARSHAL Benchmarks (50,000 Rows)
// -------------------------------------------------------------

func BenchmarkMultiType_Marshal_Struct_Sequential(b *testing.B) {
	structs, _, _ := makeMultiTypeDataset(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := Marshal(structs)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMultiType_Marshal_Generic_Sequential(b *testing.B) {
	_, generics, _ := makeMultiTypeDataset(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := MarshalSlice[MultiTypeGeneric, *MultiTypeGeneric](generics)
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMultiType_Marshal_Struct_Parallel_16W(b *testing.B) {
	structs, _, _ := makeMultiTypeDataset(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshal(structs, ParallelOptions{Workers: 16})
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}

func BenchmarkMultiType_Marshal_Generic_Parallel_16W(b *testing.B) {
	_, generics, _ := makeMultiTypeDataset(50000)
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := ParallelMarshalSlice[MultiTypeGeneric, *MultiTypeGeneric](generics, ParallelOptions{Workers: 16})
		if err != nil {
			b.Fatal(err)
		}
		b.SetBytes(int64(len(data)))
	}
}
