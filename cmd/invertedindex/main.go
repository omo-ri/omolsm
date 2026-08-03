package main

import (
	"bufio"
	"fmt"
	"omolsm/internal/invertedindex/engine"
	"omolsm/internal/invertedindex/parser"
	"os"
	"strings"
)

func main() {
	cfgPath := "configs/invertedindex.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}

	e, err := engine.NewEngineFromConfigPath(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}
	defer e.Close()

	printBanner(e)

	fmt.Println("Indexing documents...")
	if err := e.IndexSource(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to index: %v\n", err)
		os.Exit(1)
	}

	stats := e.Stats()
	fmt.Printf("Done. %d docs, %d terms indexed.\n\n", stats.DocCount, stats.TermCount)
	printHelp()

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		switch input {
		case ":quit", ":q":
			return
		case ":stats":
			printStats(e)
		case ":help":
			printHelp()
		default:
			result, err := parser.Execute(e, input)
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				continue
			}
			printResult(result)
		}
	}
}

func printBanner(e *engine.Engine) {
	conf := e.Config()
	fmt.Println("========================================")
	fmt.Println("  Inverted Index Engine")
	fmt.Println("========================================")
	fmt.Printf("  Storage backend : %s\n", conf.Storage.Backend)
	if conf.Storage.Backend == "lsm" {
		dataDir := conf.Storage.DataDir
		if dataDir == "" {
			dataDir = ".lsm-data"
		}
		fmt.Printf("  Data dir        : %s\n", dataDir)
		fmt.Printf("  Block size      : %d\n", conf.Storage.BlockSize)
		fmt.Printf("  SST size        : %d bytes\n", conf.Storage.SSTSize)
		fmt.Printf("  SST block size  : %d bytes\n", conf.Storage.SSTBlockSize)
		fmt.Printf("  SSTs per level  : %d\n", conf.Storage.SSTNumPerLevel)
		fmt.Printf("  Max levels      : %d\n", conf.Storage.MaxLevel)
	}
	fmt.Printf("  Language        : %s\n", conf.Language)
	fmt.Printf("  Source dir      : %s\n", conf.Source.Dir)
	fmt.Printf("  Stemming        : %v\n", conf.Index.Stemming)
	fmt.Printf("  Stop words      : %v\n", conf.Index.StopWords)
	fmt.Println("========================================")
	fmt.Println()
}

func printStats(e *engine.Engine) {
	s := e.Stats()
	fmt.Printf("  docs: %d, terms: %d, features: %d\n", s.DocCount, s.TermCount, s.FeatureCount)
	if report := e.LSMStatsReport(); report != "" {
		fmt.Print(report)
	}
}

func printHelp() {
	fmt.Println("Boolean queries:")
	fmt.Println("  fox                        — single term")
	fmt.Println("  fox AND dog                — both terms")
	fmt.Println("  fox OR cat                 — either term")
	fmt.Println("  fox AND NOT snake          — exclude term")
	fmt.Println("  (fox OR cat) AND NOT snake — nested")
	fmt.Println()
	fmt.Println("Prefix & wildcard (matched against stemmed terms):")
	fmt.Println("  ocea*                      — prefix search (k-gram not needed)")
	fmt.Println("  sci*ist                    — wildcard search (via k-gram index)")
	fmt.Println("  *tion                      — leading wildcard")
	fmt.Println("  m*r                        — any pattern with *")
	fmt.Println()
	fmt.Println("Date range (requires date metadata at index time):")
	fmt.Println("  DATE:[2024-01-01,2024-12-31]       — docs with Date in range")
	fmt.Println("  VALID:[2024-01-01,2024-06-30]      — docs valid in range")
	fmt.Println("  APPEARED:[2024-01-01,2024-03-31]   — docs appeared in range")
	fmt.Println("  fox AND DATE:[2024-01-01,2024-12-31] — combine text + date")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  :stats   — show index stats")
	fmt.Println("  :help    — show this help")
	fmt.Println("  :quit    — exit")
	fmt.Println()
}

func printResult(result *engine.Result) {
	docs := result.Documents()
	if len(docs) == 0 {
		fmt.Println("  No documents found.")
		return
	}
	fmt.Printf("  Found %d document(s):\n", result.Count())
	for _, doc := range docs {
		fmt.Printf("    [%d] %s\n", doc.ID, doc.Filename)
	}
}
