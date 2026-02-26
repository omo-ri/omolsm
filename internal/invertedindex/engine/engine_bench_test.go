package engine

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strconv"
	"testing"
	"time"
)

///////////////////////////////////////////////////////////////
//// CONFIG
///////////////////////////////////////////////////////////////

var benchDocSizes = []int{
	100,
	1000,
	10000,
}

var enableCPUProfile = false
var enableMemProfile = false
var exportCSV = true

///////////////////////////////////////////////////////////////
//// DATASET GENERATION
///////////////////////////////////////////////////////////////

func generateDataset(b *testing.B, numDocs int) string {

	dir := b.TempDir()

	text := `
Artificial intelligence transforms modern computing and data processing.
Scientists explore ocean ecosystems and marine biodiversity.
Climate change affects global weather patterns and ecosystems.
Space exploration expands scientific knowledge and technology.
Russian scientists develop new космических технологий и исследований.
`

	for i := 0; i < numDocs; i++ {

		filename := filepath.Join(dir,
			fmt.Sprintf("doc_%06d.txt", i))

		err := os.WriteFile(filename,
			[]byte(text), 0644)

		if err != nil {
			b.Fatal(err)
		}
	}

	return dir
}

///////////////////////////////////////////////////////////////
//// MEMORY MEASUREMENT
///////////////////////////////////////////////////////////////

func heapUsage() uint64 {

	runtime.GC()

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return m.Alloc
}

///////////////////////////////////////////////////////////////
//// DIR SIZE
///////////////////////////////////////////////////////////////

func dirSize(path string) int64 {

	var total int64

	filepath.Walk(path,
		func(_ string, info os.FileInfo, err error) error {

			if err == nil && !info.IsDir() {
				total += info.Size()
			}

			return nil
		})

	return total
}

///////////////////////////////////////////////////////////////
//// CSV EXPORT
///////////////////////////////////////////////////////////////

func exportResultsCSV(results [][]string) {

	file, err := os.Create("benchmark_results.csv")
	if err != nil {
		return
	}

	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{
		"docs",
		"data_mb",
		"time_sec",
		"docs_per_sec",
		"mb_per_sec",
		"heap_mb",
		"bytes_per_doc",
		"bytes_per_term",
	})

	for _, row := range results {
		writer.Write(row)
	}
}

///////////////////////////////////////////////////////////////
//// INDEX BENCHMARK
///////////////////////////////////////////////////////////////

func BenchmarkProductionIndex(b *testing.B) {

	var csvResults [][]string

	for _, numDocs := range benchDocSizes {

		b.Run("docs_"+strconv.Itoa(numDocs),
			func(b *testing.B) {

				dir := generateDataset(b, numDocs)

				dataSize := dirSize(dir)

				if enableCPUProfile {
					f, _ := os.Create(
						fmt.Sprintf("cpu_%d.prof", numDocs))
					pprof.StartCPUProfile(f)
					defer pprof.StopCPUProfile()
				}

				beforeMem := heapUsage()

				start := time.Now()

				engine := NewEngine()

				err := engine.IndexDir(dir)
				if err != nil {
					b.Fatal(err)
				}

				duration := time.Since(start)

				afterMem := heapUsage()

				stats := engine.Stats()

				heapDelta := afterMem - beforeMem

				docsPerSec :=
					float64(numDocs) /
						duration.Seconds()

				mbPerSec :=
					float64(dataSize) /
						duration.Seconds() /
						1024 / 1024

				bytesPerDoc :=
					heapDelta / uint64(numDocs)

				bytesPerTerm :=
					heapDelta / uint64(stats.TermCount)

				fmt.Println()
				fmt.Println("================================")
				fmt.Println("PRODUCTION INDEX BENCHMARK")
				fmt.Println("================================")

				fmt.Println("docs:", numDocs)

				fmt.Printf("data size: %.2f MB\n",
					float64(dataSize)/1024/1024)

				fmt.Println()
				fmt.Println("TIME")

				fmt.Println("duration:", duration)

				fmt.Printf("docs/sec: %.0f\n",
					docsPerSec)

				fmt.Printf("MB/sec: %.2f\n",
					mbPerSec)

				fmt.Println()
				fmt.Println("MEMORY")

				fmt.Printf("heap used: %.2f MB\n",
					float64(heapDelta)/1024/1024)

				fmt.Println("bytes/doc:",
					bytesPerDoc)

				fmt.Println("bytes/term:",
					bytesPerTerm)

				if enableMemProfile {

					f, _ := os.Create(
						fmt.Sprintf("mem_%d.prof", numDocs))

					pprof.WriteHeapProfile(f)

					f.Close()
				}

				if exportCSV {

					csvResults =
						append(csvResults,
							[]string{
								strconv.Itoa(numDocs),

								fmt.Sprintf("%.2f",
									float64(dataSize)/1024/1024),

								fmt.Sprintf("%.4f",
									duration.Seconds()),

								fmt.Sprintf("%.0f",
									docsPerSec),

								fmt.Sprintf("%.2f",
									mbPerSec),

								fmt.Sprintf("%.2f",
									float64(heapDelta)/1024/1024),

								strconv.Itoa(
									int(bytesPerDoc)),

								strconv.Itoa(
									int(bytesPerTerm)),
							})
				}
			})
	}

	if exportCSV {
		exportResultsCSV(csvResults)
	}
}

///////////////////////////////////////////////////////////////
//// QUERY BENCHMARK
///////////////////////////////////////////////////////////////

func BenchmarkProductionQuery(b *testing.B) {

	dir := generateDataset(b, 10000)

	engine := NewEngine()

	engine.IndexDir(dir)

	b.ResetTimer()

	start := time.Now()

	for i := 0; i < b.N; i++ {

		engine.Search("scientists").
			And("ocean").
			Not("climate")
	}

	duration := time.Since(start)

	qps :=
		float64(b.N) /
			duration.Seconds()

	fmt.Println()
	fmt.Println("================================")
	fmt.Println("PRODUCTION QUERY BENCHMARK")
	fmt.Println("================================")

	fmt.Printf("queries/sec: %.0f\n", qps)

	fmt.Printf("ns/query: %d\n",
		duration.Nanoseconds()/int64(b.N))
}
