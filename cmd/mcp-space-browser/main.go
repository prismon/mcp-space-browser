package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/prismon/mcp-space-browser/internal/models"
	"github.com/prismon/mcp-space-browser/pkg/crawler"
	"github.com/prismon/mcp-space-browser/pkg/database"
	"github.com/prismon/mcp-space-browser/pkg/logger"
	"github.com/prismon/mcp-space-browser/pkg/server"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	log *logrus.Entry

	// Tree command options
	sortBy    string
	ascending bool
	minDate   string
	maxDate   string

	// Server command options
	port int

	// Database path
	dbPath string
)

func init() {
	log = logger.WithName("cli")
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
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "disk.db", "Path to SQLite database file")

	// disk-index command
	var diskIndexCmd = &cobra.Command{
		Use:   "disk-index <path>",
		Short: "Index a directory tree",
		Args:  cobra.ExactArgs(1),
		Run:   runDiskIndex,
	}

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

	serverCmd.Flags().IntVar(&port, "port", 3000, "Port to listen on")

	rootCmd.AddCommand(diskIndexCmd, diskDuCmd, diskTreeCmd, serverCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runDiskIndex(cmd *cobra.Command, args []string) {
	target := args[0]
	log.WithFields(logrus.Fields{
		"command": "disk-index",
		"target":  target,
	}).Info("Executing command")

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := crawler.Index(target, db); err != nil {
		log.WithError(err).Error("Failed to index directory")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
	log.WithField("port", port).Info("Starting HTTP server")

	db, err := database.NewDiskDB(dbPath)
	if err != nil {
		log.WithError(err).Error("Failed to open database")
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := server.Start(port, db); err != nil {
		log.WithError(err).Error("Server failed")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
