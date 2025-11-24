package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/auth"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/home"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	log *logrus.Entry

	// Global flags
	homeDir     string
	homeManager *home.Manager

	// Tree command options
	sortBy    string
	ascending bool
	minDate   string
	maxDate   string

	// Server command options
	port         int
	host         string
	externalHost string
	configPath   string

	// Database path
	dbPath string

	// Index command options
	parallel    bool
	workerCount int
	queueSize   int
	batchSize   int
)

func init() {
	log = logger.WithName("cli")
}

// initHome initializes the home directory manager
func initHome() error {
	var err error
	homeManager, err = home.NewManager(homeDir)
	if err != nil {
		return fmt.Errorf("failed to initialize home: %w", err)
	}

	// Initialize home directory if it doesn't exist
	if !homeManager.Exists() {
		log.Info("Initializing home directory: " + homeManager.Path())
		if err := homeManager.Initialize(); err != nil {
			return fmt.Errorf("failed to initialize home directory: %w", err)
		}
	}

	return nil
}

func main() {
	var rootCmd = &cobra.Command{
		Use:   "mcp-space-browser",
		Short: "Disk space indexing and analysis tool",
		Long: `mcp-space-browser - Disk space indexing agent built with Go.

It crawls filesystems, stores metadata in SQLite, and provides tools for
exploring disk utilization (similar to Baobab/WinDirStat).`,
	}

	// Add global flags
	rootCmd.PersistentFlags().StringVar(&homeDir, "home", "", "Home directory for mcp-space-browser (default: ~/.mcp-space-browser)")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to SQLite database file (default: <home>/disk.db)")

	// disk-index command
	var diskIndexCmd = &cobra.Command{
		Use:   "disk-index <path>",
		Short: "Index a directory tree",
		Args:  cobra.ExactArgs(1),
		Run:   runDiskIndex,
	}

	diskIndexCmd.Flags().BoolVar(&parallel, "parallel", false, "Use parallel indexing with worker pool")
	diskIndexCmd.Flags().IntVar(&workerCount, "workers", 8, "Number of parallel workers (only with --parallel)")
	diskIndexCmd.Flags().IntVar(&queueSize, "queue-size", 10000, "Size of job queue (only with --parallel)")
	diskIndexCmd.Flags().IntVar(&batchSize, "batch-size", 1000, "Database write batch size (only with --parallel)")

	// disk-du command
	var diskDuCmd = &cobra.Command{
		Use:   "disk-du <path>",
		Short: "Show disk usage for a path",
		Args:  cobra.ExactArgs(1),
		Run:   runDiskDu,
	}

	// disk-tree command
	var diskTreeCmd = &cobra.Command{
		Use:   "disk-tree <path>",
		Short: "Display tree view with sizes",
		Args:  cobra.ExactArgs(1),
		Run:   runDiskTree,
	}

	diskTreeCmd.Flags().StringVar(&sortBy, "sort-by", "", "Sort by size, mtime, or name")
	diskTreeCmd.Flags().BoolVar(&ascending, "ascending", false, "Sort in ascending order (default: descending)")
	diskTreeCmd.Flags().StringVar(&minDate, "min-date", "", "Filter files modified after this date (YYYY-MM-DD)")
	diskTreeCmd.Flags().StringVar(&maxDate, "max-date", "", "Filter files modified before this date (YYYY-MM-DD)")

	// server command
	var serverCmd = &cobra.Command{
		Use:   "server",
		Short: "Start HTTP server",
		Run:   runServer,
	}

	serverCmd.Flags().IntVar(&port, "port", 0, "Port to listen on (overrides config file, 0 = use config)")
	serverCmd.Flags().StringVar(&host, "host", "", "Host address to bind to (overrides config file, empty = use config)")
	serverCmd.Flags().StringVar(&configPath, "config", "config.yaml", "Path to configuration file")
	serverCmd.Flags().StringVar(&externalHost, "external-host", "", "External hostname/URL for generating resource URLs. Can be a hostname (e.g., 'example.com') or full URL (e.g., 'https://example.com'). Defaults to --host, or localhost if binding to 0.0.0.0")

	// job-list command
	var jobListCmd = &cobra.Command{
		Use:   "job-list",
		Short: "List all indexing jobs",
		Run:   runJobList,
	}

	// job-status command
	var jobStatusCmd = &cobra.Command{
		Use:   "job-status <job-id>",
		Short: "Get status of an indexing job",
		Args:  cobra.ExactArgs(1),
		Run:   runJobStatus,
	}

	// home-init command
	var homeInitCmd = &cobra.Command{
		Use:   "home-init",
		Short: "Initialize or reinitialize the home directory",
		Long: `Creates the home directory structure with default configuration,
example rules, and necessary subdirectories.`,
		Run: runHomeInit,
	}

	// home-info command
	var homeInfoCmd = &cobra.Command{
		Use:   "home-info",
		Short: "Display information about the home directory",
		Run:   runHomeInfo,
	}

	// home-clean command
	var homeCleanCmd = &cobra.Command{
		Use:   "home-clean",
		Short: "Clean temporary files and optionally cache",
		Run:   runHomeClean,
	}

	homeCleanCmd.Flags().Bool("cache", false, "Also clean cache directory")

	rootCmd.AddCommand(diskIndexCmd, diskDuCmd, diskTreeCmd, serverCmd, jobListCmd, jobStatusCmd, homeInitCmd, homeInfoCmd, homeCleanCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runDiskIndex(cmd *cobra.Command, args []string) {
	target := args[0]
	log.WithFields(logrus.Fields{
		"command":  "disk-index",
		"target":   target,
		"parallel": parallel,
	}).Info("Executing command")

	dbPath, err := getDBPath()
	if err != nil {
		log.WithError(err).Error("Failed to get database path")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create stdout progress callback
	progressCallback := func(stats *crawler.IndexStats, remaining int) {
		fmt.Printf("\rIndexing: %d files, %d directories (%.2f MB) - %d remaining",
			stats.FilesProcessed, stats.DirectoriesProcessed,
			float64(stats.TotalSize)/(1024*1024), remaining)
	}

	if parallel {
		// Use parallel indexing
		opts := &crawler.ParallelIndexOptions{
			WorkerCount:      workerCount,
			QueueSize:        queueSize,
			BatchSize:        batchSize,
			ProgressCallback: progressCallback,
		}

		log.WithFields(logrus.Fields{
			"workerCount": opts.WorkerCount,
			"queueSize":   opts.QueueSize,
			"batchSize":   opts.BatchSize,
		}).Info("Starting parallel indexing")

		fmt.Printf("Starting indexing of %s...\n", target)
		stats, err := crawler.IndexParallel(target, db, nil, opts)
		if err != nil {
			fmt.Println() // Clear progress line
			log.WithError(err).Error("Failed to index directory")
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nIndexed %d files and %d directories (%.2f MB) in %s\n",
			stats.FilesProcessed, stats.DirectoriesProcessed,
			float64(stats.TotalSize)/(1024*1024), stats.Duration)
	} else {
		// Use sequential indexing (no job tracking for CLI)
		fmt.Printf("Starting indexing of %s...\n", target)
		stats, err := crawler.Index(target, db, nil, 0, progressCallback)
		if err != nil {
			fmt.Println() // Clear progress line
			log.WithError(err).Error("Failed to index directory")
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("\nIndexed %d files and %d directories (%.2f MB) in %s\n",
			stats.FilesProcessed, stats.DirectoriesProcessed,
			float64(stats.TotalSize)/(1024*1024), stats.Duration)
	}

	log.WithFields(logrus.Fields{
		"command": "disk-index",
		"target":  target,
	}).Info("Command completed successfully")
}

func runDiskDu(cmd *cobra.Command, args []string) {
	target := args[0]
	log.WithFields(logrus.Fields{
		"command": "disk-du",
		"target":  target,
	}).Info("Executing command")

	dbPath, err := getDBPath()
	if err != nil {
		log.WithError(err).Error("Failed to get database path")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	abs, err := filepath.Abs(target)
	if err != nil {
		log.WithError(err).Error("Failed to resolve absolute path")
		fmt.Fprintf(os.Stderr, "Error: Failed to resolve path: %v\n", err)
		os.Exit(1)
	}

	entry, err := db.Get(abs)
	if err != nil {
		log.WithError(err).Error("Failed to get entry")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if entry == nil {
		log.WithFields(logrus.Fields{
			"command": "disk-du",
			"target":  target,
		}).Warn("Path not found in database")
		fmt.Fprintf(os.Stderr, "Error: Path '%s' not found in database. Run 'disk-index %s' first.\n", target, target)
		os.Exit(1)
	}

	log.WithFields(logrus.Fields{
		"command": "disk-du",
		"target":  target,
		"size":    entry.Size,
	}).Info("Disk usage calculated")

	fmt.Println(entry.Size)
}

type treeOptions struct {
	sortBy    string
	ascending bool
	minDate   *time.Time
	maxDate   *time.Time
}

func runDiskTree(cmd *cobra.Command, args []string) {
	target := args[0]
	log.WithFields(logrus.Fields{
		"command": "disk-tree",
		"target":  target,
	}).Info("Executing command")

	dbPath, err := getDBPath()
	if err != nil {
		log.WithError(err).Error("Failed to get database path")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	opts := &treeOptions{
		sortBy:    sortBy,
		ascending: ascending,
	}

	if minDate != "" {
		t, err := time.Parse("2006-01-02", minDate)
		if err != nil {
			log.WithError(err).Error("Invalid min-date format")
			fmt.Fprintf(os.Stderr, "Error: Invalid min-date format (expected YYYY-MM-DD): %v\n", err)
			os.Exit(1)
		}
		opts.minDate = &t
	}

	if maxDate != "" {
		t, err := time.Parse("2006-01-02", maxDate)
		if err != nil {
			log.WithError(err).Error("Invalid max-date format")
			fmt.Fprintf(os.Stderr, "Error: Invalid max-date format (expected YYYY-MM-DD): %v\n", err)
			os.Exit(1)
		}
		opts.maxDate = &t
	}

	if err := diskTree(db, target, "", true, opts); err != nil {
		log.WithError(err).Error("Failed to display tree")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	log.WithFields(logrus.Fields{
		"command": "disk-tree",
		"target":  target,
	}).Info("Tree display completed")
}

func diskTree(db *database.DiskDB, target string, indent string, isRoot bool, opts *treeOptions) error {
	abs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	entry, err := db.Get(abs)
	if err != nil {
		return fmt.Errorf("failed to get entry: %w", err)
	}

	if entry == nil {
		log.WithField("path", abs).Warn("Entry not found")
		return nil
	}

	// Apply date filtering (but always show root)
	if !isRoot {
		if opts.minDate != nil && entry.Mtime < opts.minDate.Unix() {
			return nil
		}
		if opts.maxDate != nil && entry.Mtime > opts.maxDate.Unix() {
			return nil
		}
	}

	mtimeStr := time.Unix(entry.Mtime, 0).Format("2006-01-02")
	fmt.Printf("%s%s (%d) [%s]\n", indent, filepath.Base(abs), entry.Size, mtimeStr)

	children, err := db.Children(abs)
	if err != nil {
		return fmt.Errorf("failed to get children: %w", err)
	}

	// Apply date filtering to children
	if opts.minDate != nil || opts.maxDate != nil {
		var filtered []*models.Entry
		for _, child := range children {
			if opts.minDate != nil && child.Mtime < opts.minDate.Unix() {
				continue
			}
			if opts.maxDate != nil && child.Mtime > opts.maxDate.Unix() {
				continue
			}
			filtered = append(filtered, child)
		}
		children = filtered
	}

	// Sort children
	if opts.sortBy != "" {
		sort.Slice(children, func(i, j int) bool {
			var comparison bool
			switch opts.sortBy {
			case "size":
				comparison = children[i].Size < children[j].Size
			case "mtime":
				comparison = children[i].Mtime < children[j].Mtime
			case "name":
				comparison = filepath.Base(children[i].Path) < filepath.Base(children[j].Path)
			default:
				comparison = false
			}
			if opts.ascending {
				return comparison
			}
			return !comparison
		})
	}

	log.WithFields(logrus.Fields{
		"path":       abs,
		"childCount": len(children),
	}).Trace("Processing children for tree")

	for _, child := range children {
		if err := diskTree(db, child.Path, indent+"  ", false, opts); err != nil {
			return err
		}
	}

	return nil
}

func runServer(cmd *cobra.Command, args []string) {
	// Initialize home if not already done (for config path resolution)
	if configPath == "" && homeManager == nil {
		if err := initHome(); err != nil {
			log.WithError(err).Error("Failed to initialize home")
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		configPath = homeManager.ConfigPath()
	}

	// Load configuration
	config, err := auth.LoadConfig(configPath)
	if err != nil {
		log.WithError(err).Error("Failed to load configuration")
		fmt.Fprintf(os.Stderr, "Error: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Configure logger from config (environment variables already override config.Logging.Level)
	if err := logger.ConfigureFromString(config.Logging.Level); err != nil {
		log.WithError(err).Warn("Invalid log level in configuration, using default")
	} else {
		log.WithField("level", config.Logging.Level).Debug("Logger configured from config")
	}

	// Determine the external host if not specified
	effectiveExternalHost := externalHost
	if effectiveExternalHost == "" {
		if host == "0.0.0.0" || host == "" || host == "::" {
			effectiveExternalHost = "localhost"
		} else {
			effectiveExternalHost = host
		}
	}

	log.WithFields(logrus.Fields{
		"port":         port,
		"host":         host,
		"externalHost": effectiveExternalHost,
	}).Info("Starting unified HTTP server")

	// Override config with command-line flags
	if port > 0 {
		config.Server.Port = port
	}
	if host != "" {
		config.Server.Host = host
	}
	if externalHost != "" {
		config.Server.ExternalHost = externalHost
	}
	if dbPath != "" {
		config.Database.Path = dbPath
	} else if homeManager != nil {
		// Use home-based database path if not specified
		config.Database.Path = homeManager.DatabasePath()
	}

	// Determine the external host if not specified
	effectiveExternalHost = config.Server.ExternalHost
	if effectiveExternalHost == "" {
		if config.Server.Host == "0.0.0.0" || config.Server.Host == "" || config.Server.Host == "::" {
			effectiveExternalHost = "localhost"
		} else {
			effectiveExternalHost = config.Server.Host
		}
		config.Server.ExternalHost = effectiveExternalHost
	}

	// Update base URL from host/port if not explicitly set
	if config.Server.BaseURL == "" || config.Server.BaseURL == "http://localhost:3000" {
		config.Server.BaseURL = fmt.Sprintf("http://%s:%d", effectiveExternalHost, config.Server.Port)
	}

	log.WithFields(logrus.Fields{
		"port":         config.Server.Port,
		"host":         config.Server.Host,
		"externalHost": effectiveExternalHost,
		"config_file":  configPath,
		"auth_enabled": config.Auth.Enabled,
	}).Info("Starting unified HTTP server")

	// Open database
	db, err := database.NewDiskDB(config.Database.Path)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()


	if err := server.Start(config, db, config.Database.Path); err != nil {
		log.WithError(err).Error("Server failed")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runJobList(cmd *cobra.Command, args []string) {
	log.Info("Listing indexing jobs")

	dbPath, err := getDBPath()
	if err != nil {
		log.WithError(err).Error("Failed to get database path")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	jobs, err := db.ListIndexJobs(nil, 50)
	if err != nil {
		log.WithError(err).Error("Failed to list jobs")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(jobs) == 0 {
		fmt.Println("No indexing jobs found")
		return
	}

	fmt.Printf("%-5s %-50s %-12s %-8s %-20s\n", "ID", "Path", "Status", "Progress", "Created")
	fmt.Println("--------------------------------------------------------------------------------")

	for _, job := range jobs {
		createdAt := time.Unix(job.CreatedAt, 0).Format("2006-01-02 15:04:05")
		fmt.Printf("%-5d %-50s %-12s %-7d%% %-20s\n",
			job.ID,
			truncateString(job.RootPath, 50),
			job.Status,
			job.Progress,
			createdAt,
		)
	}
}

func runJobStatus(cmd *cobra.Command, args []string) {
	jobIDStr := args[0]
	jobID, err := parseInt64(jobIDStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid job ID: %v\n", err)
		os.Exit(1)
	}

	log.WithField("jobID", jobID).Info("Getting job status")

	dbPath, err := getDBPath()
	if err != nil {
		log.WithError(err).Error("Failed to get database path")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	job, err := db.GetIndexJob(jobID)
	if err != nil {
		log.WithError(err).Error("Failed to get job")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if job == nil {
		fmt.Fprintf(os.Stderr, "Error: Job %d not found\n", jobID)
		os.Exit(1)
	}

	fmt.Printf("Job ID: %d\n", job.ID)
	fmt.Printf("Root Path: %s\n", job.RootPath)
	fmt.Printf("Status: %s\n", job.Status)
	fmt.Printf("Progress: %d%%\n", job.Progress)
	fmt.Printf("Created: %s\n", time.Unix(job.CreatedAt, 0).Format("2006-01-02 15:04:05"))

	if job.StartedAt != nil {
		fmt.Printf("Started: %s\n", time.Unix(*job.StartedAt, 0).Format("2006-01-02 15:04:05"))
	}

	if job.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", time.Unix(*job.CompletedAt, 0).Format("2006-01-02 15:04:05"))

		if job.StartedAt != nil {
			duration := time.Unix(*job.CompletedAt, 0).Sub(time.Unix(*job.StartedAt, 0))
			fmt.Printf("Duration: %s\n", duration)
		}
	}

	if job.Error != nil {
		fmt.Printf("Error: %s\n", *job.Error)
	}

	if job.Metadata != nil {
		fmt.Printf("\nMetadata:\n%s\n", *job.Metadata)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func parseInt64(s string) (int64, error) {
	var i int64
	_, err := fmt.Sscanf(s, "%d", &i)
	return i, err
}

// getDBPath returns the database path, using home directory if dbPath is not specified
func getDBPath() (string, error) {
	if dbPath != "" {
		return dbPath, nil
	}

	// Initialize home if not already done
	if homeManager == nil {
		if err := initHome(); err != nil {
			return "", err
		}
	}

	return homeManager.DatabasePath(), nil
}

func runHomeInit(cmd *cobra.Command, args []string) {
	if err := initHome(); err != nil {
		log.WithError(err).Error("Failed to initialize home directory")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Force reinitialize
	if err := homeManager.Initialize(); err != nil {
		log.WithError(err).Error("Failed to initialize home directory")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Home directory initialized at: %s\n", homeManager.Path())
	fmt.Printf("\nDirectory structure:\n")
	fmt.Printf("  Config: %s\n", homeManager.ConfigPath())
	fmt.Printf("  Database: %s\n", homeManager.DatabasePath())
	fmt.Printf("  Rules (enabled): %s\n", homeManager.RulesEnabledPath())
	fmt.Printf("  Rules (disabled): %s\n", homeManager.RulesDisabledPath())
	fmt.Printf("  Rules (examples): %s\n", homeManager.RulesExamplesPath())
	fmt.Printf("  Cache: %s\n", homeManager.JoinPath(home.CacheDir))
	fmt.Printf("  Temp: %s\n", homeManager.TempPath())
	fmt.Printf("  Logs: %s\n", homeManager.LogsPath())
}

func runHomeInfo(cmd *cobra.Command, args []string) {
	if err := initHome(); err != nil {
		log.WithError(err).Error("Failed to initialize home directory")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cacheSize, err := homeManager.GetCacheSize()
	if err != nil {
		log.WithError(err).Warn("Failed to calculate cache size")
		cacheSize = 0
	}

	fmt.Printf("Home Directory: %s\n", homeManager.Path())
	fmt.Printf("Exists: %v\n\n", homeManager.Exists())

	fmt.Printf("Paths:\n")
	fmt.Printf("  Config: %s\n", homeManager.ConfigPath())
	fmt.Printf("  Database: %s\n", homeManager.DatabasePath())
	fmt.Printf("  Rules (enabled): %s\n", homeManager.RulesEnabledPath())
	fmt.Printf("  Rules (disabled): %s\n", homeManager.RulesDisabledPath())
	fmt.Printf("  Rules (examples): %s\n", homeManager.RulesExamplesPath())
	fmt.Printf("  Cache: %s\n", homeManager.JoinPath(home.CacheDir))
	fmt.Printf("  Temp: %s\n", homeManager.TempPath())
	fmt.Printf("  Logs: %s\n\n", homeManager.LogsPath())

	fmt.Printf("Cache Size: %d bytes (%.2f MB)\n", cacheSize, float64(cacheSize)/(1024*1024))

	// Try to load config
	config, err := homeManager.LoadConfig()
	if err != nil {
		fmt.Printf("\nConfiguration: Error loading config: %v\n", err)
	} else {
		fmt.Printf("\nConfiguration:\n")
		fmt.Printf("  Server Port: %d\n", config.Server.Port)
		fmt.Printf("  Server Host: %s\n", config.Server.Host)
		fmt.Printf("  Rules Auto-Execute: %v\n", config.Rules.AutoExecute)
		fmt.Printf("  Rules Hot-Reload: %v\n", config.Rules.HotReload)
		fmt.Printf("  Cache Enabled: %v\n", config.Cache.Enabled)
		fmt.Printf("  Cache Max Size: %.2f GB\n", float64(config.Cache.MaxSize)/(1024*1024*1024))
		fmt.Printf("  Log Level: %s\n", config.Logging.Level)
	}
}

func runHomeClean(cmd *cobra.Command, args []string) {
	if err := initHome(); err != nil {
		log.WithError(err).Error("Failed to initialize home directory")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cleanCache, _ := cmd.Flags().GetBool("cache")

	// Clean temp directory
	if err := homeManager.CleanTemp(); err != nil {
		log.WithError(err).Error("Failed to clean temp directory")
		fmt.Fprintf(os.Stderr, "Error cleaning temp: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Cleaned temp directory")

	// Clean cache if requested
	if cleanCache {
		cachePath := homeManager.JoinPath(home.CacheDir)
		if err := os.RemoveAll(cachePath); err != nil {
			log.WithError(err).Error("Failed to clean cache")
			fmt.Fprintf(os.Stderr, "Error cleaning cache: %v\n", err)
			os.Exit(1)
		}
		if err := os.MkdirAll(cachePath, 0755); err != nil {
			log.WithError(err).Error("Failed to recreate cache directory")
			fmt.Fprintf(os.Stderr, "Error recreating cache: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Cleaned cache directory")
	}

	fmt.Println("\nCleanup complete!")
}
