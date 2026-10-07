package csv

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"unique"
	"unsafe"
)

type InternRecord struct {
	ID       unique.Handle[int]     `csv:"id"`
	Status   unique.Handle[string]  `csv:"status"`
	Category string                 `csv:"category,unique"`
	Region   string                 `csv:"region,intern"`
	Score    unique.Handle[float64] `csv:"score"`
	Active   unique.Handle[bool]    `csv:"active"`
	Plain    string                 `csv:"plain"`
}

func TestUniqueUnmarshalAndMarshal(t *testing.T) {
	csvData := `id,status,category,region,score,active,plain
1,ACTIVE,BOOKS,NA,98.5,true,first
2,ACTIVE,BOOKS,NA,98.5,true,second
3,PENDING,ELECTRONICS,EU,50,false,third
`
	var records []InternRecord
	if err := Unmarshal([]byte(csvData), &records); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// 1. Verify unique.Handle equality (O(1) pointer comparison)
	if records[0].Status != records[1].Status {
		t.Errorf("records[0].Status (%s) != records[1].Status (%s)", records[0].Status.Value(), records[1].Status.Value())
	}
	if records[0].Status.Value() != "ACTIVE" {
		t.Errorf("expected ACTIVE, got %q", records[0].Status.Value())
	}
	if records[2].Status.Value() != "PENDING" {
		t.Errorf("expected PENDING, got %q", records[2].Status.Value())
	}
	if records[0].Status == records[2].Status {
		t.Errorf("records[0].Status should not equal records[2].Status")
	}

	// 2. Verify unique.Handle primitive types
	if records[0].ID.Value() != 1 || records[1].ID.Value() != 2 {
		t.Errorf("unexpected IDs: %d, %d", records[0].ID.Value(), records[1].ID.Value())
	}
	if records[0].Score != records[1].Score || records[0].Score.Value() != 98.5 {
		t.Errorf("unexpected score handle")
	}
	if !records[0].Active.Value() || records[2].Active.Value() {
		t.Errorf("unexpected bool handle")
	}

	// 3. Verify string interning: unsafe.StringData must point to the identical canonical memory!
	ptr0 := unsafe.StringData(records[0].Category)
	ptr1 := unsafe.StringData(records[1].Category)
	if ptr0 != ptr1 {
		t.Errorf("expected interned Category to share string memory: ptr0=%p, ptr1=%p", ptr0, ptr1)
	}
	if records[0].Category != "BOOKS" {
		t.Errorf("expected BOOKS, got %q", records[0].Category)
	}

	// Region tagged with "intern"
	regPtr0 := unsafe.StringData(records[0].Region)
	regPtr1 := unsafe.StringData(records[1].Region)
	if regPtr0 != regPtr1 {
		t.Errorf("expected interned Region to share string memory: regPtr0=%p, regPtr1=%p", regPtr0, regPtr1)
	}
	if records[0].Region != "NA" {
		t.Errorf("expected NA, got %q", records[0].Region)
	}

	// Plain field (NOT interned): each row allocates its own string slice
	plainPtr0 := unsafe.StringData(records[0].Plain)
	plainPtr1 := unsafe.StringData(records[1].Plain)
	if plainPtr0 == plainPtr1 {
		t.Errorf("plain strings should not share pointers")
	}

	// 4. Test Marshal
	marshaled, err := Marshal(records)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	expected := "id,status,category,region,score,active,plain\n" +
		"1,ACTIVE,BOOKS,NA,98.5,true,first\n" +
		"2,ACTIVE,BOOKS,NA,98.5,true,second\n" +
		"3,PENDING,ELECTRONICS,EU,50,false,third\n"
	if string(marshaled) != expected {
		t.Errorf("Marshal output mismatch:\ngot:\n%s\nwant:\n%s", string(marshaled), expected)
	}
}

func TestUniqueParallel(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("id,status,category,region,score,active,plain\n")
	const numRows = 2000
	for i := 0; i < numRows; i++ {
		status := "ACTIVE"
		if i%3 == 0 {
			status = "PENDING"
		} else if i%3 == 1 {
			status = "SUSPENDED"
		}
		sb.WriteString(fmt.Sprintf("%d,%s,ELECTRONICS,APAC,100,true,row%d\n", i, status, i))
	}
	csvBytes := []byte(sb.String())

	var records []InternRecord
	err := ParallelUnmarshal(csvBytes, &records, ParallelOptions{Workers: 4})
	if err != nil {
		t.Fatalf("ParallelUnmarshal failed: %v", err)
	}
	if len(records) != numRows {
		t.Fatalf("expected %d rows, got %d", numRows, len(records))
	}

	// Verify all Category="ELECTRONICS" across different chunks share the same StringData pointer
	firstCategoryPtr := unsafe.StringData(records[0].Category)
	for i := 1; i < numRows; i++ {
		ptr := unsafe.StringData(records[i].Category)
		if ptr != firstCategoryPtr {
			t.Fatalf("chunk worker string interning failed at row %d: ptr=%p vs first=%p", i, ptr, firstCategoryPtr)
		}
	}

	// Verify ParallelMarshal
	out, err := ParallelMarshal(records, ParallelOptions{Workers: 4})
	if err != nil {
		t.Fatalf("ParallelMarshal failed: %v", err)
	}
	if !bytes.Equal(out, csvBytes) {
		t.Fatalf("ParallelMarshal output mismatch (len %d vs %d)", len(out), len(csvBytes))
	}
}

func TestUniqueStreamingDecoderEncoder(t *testing.T) {
	csvData := `id,status,category,region,score,active,plain
10,PAID,FINANCE,US,12.34,true,line1
20,PAID,FINANCE,US,56.78,false,line2
`
	dec, err := NewDecoder(strings.NewReader(csvData))
	if err != nil {
		t.Fatalf("NewDecoder failed: %v", err)
	}

	var rows []InternRecord
	for {
		var row InternRecord
		err := dec.Decode(&row)
		if err != nil {
			if strings.Contains(err.Error(), "EOF") {
				break
			}
			t.Fatalf("Decode failed: %v", err)
		}
		rows = append(rows, row)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].Status != rows[1].Status {
		t.Errorf("streaming handles should be equal")
	}
	if unsafe.StringData(rows[0].Category) != unsafe.StringData(rows[1].Category) {
		t.Errorf("streaming interned strings should share string data")
	}

	var buf bytes.Buffer
	enc := NewEncoder(&buf)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
			t.Fatalf("Encode failed: %v", err)
		}
	}
	if err := enc.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	expected := "id,status,category,region,score,active,plain\n" +
		"10,PAID,FINANCE,US,12.34,true,line1\n" +
		"20,PAID,FINANCE,US,56.78,false,line2\n"
	if buf.String() != expected {
		t.Errorf("Encoder output mismatch:\ngot:\n%s\nwant:\n%s", buf.String(), expected)
	}
}

// Benchmark comparing memory allocations with and without string interning
type RepetitivePlain struct {
	ID       int    `csv:"id"`
	Status   string `csv:"status"`
	Category string `csv:"category"`
	Region   string `csv:"region"`
}

type RepetitiveInterned struct {
	ID       int    `csv:"id"`
	Status   string `csv:"status,unique"`
	Category string `csv:"category,unique"`
	Region   string `csv:"region,unique"`
}

type RepetitiveHandle struct {
	ID       int                   `csv:"id"`
	Status   unique.Handle[string] `csv:"status"`
	Category unique.Handle[string] `csv:"category"`
	Region   unique.Handle[string] `csv:"region"`
}

func generateRepetitiveCSV(n int) []byte {
	var sb strings.Builder
	sb.WriteString("id,status,category,region\n")
	statuses := []string{"ACTIVE", "PENDING", "SUSPENDED", "ARCHIVED", "CLOSED"}
	categories := []string{"ELECTRONICS", "BOOKS", "CLOTHING", "HOME", "BEAUTY", "GARDEN", "SPORTS"}
	regions := []string{"NORTH_AMERICA", "EUROPE", "ASIA_PACIFIC", "LATIN_AMERICA"}

	for i := 0; i < n; i++ {
		sb.WriteString(fmt.Sprintf("%d,%s,%s,%s\n",
			i,
			statuses[i%len(statuses)],
			categories[i%len(categories)],
			regions[i%len(regions)],
		))
	}
	return []byte(sb.String())
}

func BenchmarkUnmarshal_RepetitivePlain(b *testing.B) {
	data := generateRepetitiveCSV(10000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var list []RepetitivePlain
		if err := Unmarshal(data, &list); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_RepetitiveInterned(b *testing.B) {
	data := generateRepetitiveCSV(10000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var list []RepetitiveInterned
		if err := Unmarshal(data, &list); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshal_RepetitiveHandle(b *testing.B) {
	data := generateRepetitiveCSV(10000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var list []RepetitiveHandle
		if err := Unmarshal(data, &list); err != nil {
			b.Fatal(err)
		}
	}
}
