package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prismon/mcp-space-browser/pkg/classifier"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
)

func main() {
	dbPath := flag.String("db", "./disk.db", "Database path")
	cacheDir := flag.String("cache", "cache", "Cache directory")
	workers := flag.Int("workers", 8, "Number of parallel workers")
	inputFile := flag.String("input", "", "File with paths to process (one per line)")
	artifactTypes := flag.String("types", "thumbnail", "Artifact types (comma-separated)")
	flag.Parse()

	if *inputFile == "" {
		fmt.Println("Usage: batch-classify -input <file> [-db <path>] [-cache <dir>] [-workers <n>]")
		os.Exit(1)
	}

	log := logger.WithName("batch-classify")
	log.Info("Starting batch classification")

	// Initialize database
	db, err := database.NewDiskDB(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Initialize classifier
	processor := classifier.NewProcessor(&classifier.ProcessorConfig{
		CacheDir:          *cacheDir,
		ClassifierManager: classifier.NewManager(),
		MetadataManager:   classifier.NewMetadataManager(),
		Database:          db,
	})

	// Read input file
	file, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	defer file.Close()

	// Collect all paths
	var paths []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		path := scanner.Text()
		if path != "" {
			paths = append(paths, path)
		}
	}

	total := len(paths)
	log.Infof("Processing %d files with %d workers", total, *workers)

	// Process in parallel
	var processed int64
	var succeeded int64
	var failed int64
	startTime := time.Now()

	pathChan := make(chan string, *workers*2)
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range pathChan {
				req := &classifier.ProcessRequest{
					ResourceURL:   "file://" + path,
					ArtifactTypes: []string{*artifactTypes},
				}

				_, err := processor.ProcessResource(req)
				atomic.AddInt64(&processed, 1)
				if err != nil {
					atomic.AddInt64(&failed, 1)
				} else {
					atomic.AddInt64(&succeeded, 1)
				}

				// Progress every 100 files
				p := atomic.LoadInt64(&processed)
				if p%100 == 0 {
					elapsed := time.Since(startTime)
					rate := float64(p) / elapsed.Seconds()
					remaining := time.Duration(float64(int64(total)-p)/rate) * time.Second
					fmt.Printf("\rProcessed: %d/%d (%.1f/s) - Success: %d, Failed: %d - ETA: %v     ",
						p, total, rate, atomic.LoadInt64(&succeeded), atomic.LoadInt64(&failed), remaining.Round(time.Second))
				}
			}
		}()
	}

	// Send paths to workers
	for _, path := range paths {
		pathChan <- path
	}
	close(pathChan)

	// Wait for completion
	wg.Wait()

	elapsed := time.Since(startTime)
	fmt.Printf("\n\nCompleted in %v\n", elapsed.Round(time.Second))
	fmt.Printf("Total: %d, Success: %d, Failed: %d\n", processed, succeeded, failed)
	fmt.Printf("Rate: %.1f files/sec\n", float64(processed)/elapsed.Seconds())
}
