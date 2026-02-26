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

	e, err := engine.NewEngineFromConfig(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

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
			s := e.Stats()
			fmt.Printf("  docs: %d, terms: %d, features: %d\n", s.DocCount, s.TermCount, s.FeatureCount)
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

func printHelp() {
	fmt.Println("Queries:")
	fmt.Println("  fox                        — single term")
	fmt.Println("  fox AND dog                — both terms")
	fmt.Println("  fox OR cat                 — either term")
	fmt.Println("  fox AND NOT snake          — exclude term")
	fmt.Println("  (fox OR cat) AND NOT snake — nested")
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
