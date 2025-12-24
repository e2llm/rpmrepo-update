package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/e2llm/rpmrepo-update/pkg/backend"
	"github.com/e2llm/rpmrepo-update/pkg/repo"
)

var version = "dev"

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	root := flag.NewFlagSet("rpmrepo-update", flag.ContinueOnError)
	root.SetOutput(os.Stderr)

	var backendType string
	var repoRoot string
	var logLevel string
	var outputFormat string
	var showVersion bool
	var signRepodata bool
	var gpgKey string
	var signRPMs bool
	var s3Endpoint string
	root.StringVar(&backendType, "backend", "fs", "backend to use (fs, s3)")
	root.StringVar(&repoRoot, "repo-root", "", "repository root path or URI")
	root.StringVar(&logLevel, "log-level", "info", "log level (info, debug)")
	root.StringVar(&outputFormat, "output", "text", "output format for commands that support it (text, json)")
	root.BoolVar(&showVersion, "version", false, "print version and exit")
	root.BoolVar(&signRepodata, "sign-repodata", false, "sign repomd.xml with gpg (requires --gpg-key or default key)")
	root.StringVar(&gpgKey, "gpg-key", "", "GPG key ID to use when signing (default: gpg defaults)")
	root.BoolVar(&signRPMs, "sign-rpms", false, "re-sign RPMs before adding (GPG)")
	root.StringVar(&s3Endpoint, "s3-endpoint", "", "S3 endpoint URL for S3-compatible storage (e.g., MinIO)")
	root.Usage = func() {
		fmt.Fprintf(root.Output(), "Usage: rpmrepo-update [global flags] <command> [args]\n")
		fmt.Fprintf(root.Output(), "Commands: init, add, remove, check\n\n")
		root.PrintDefaults()
	}

	if err := root.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if showVersion {
		fmt.Fprintf(os.Stdout, "%s\n", version)
		return nil
	}

	remaining := root.Args()
	if len(remaining) == 0 {
		root.Usage()
		return fmt.Errorf("missing command")
	}

	switch remaining[0] {
	case "init":
		return runInit(ctx, backendType, repoRoot, s3Endpoint, logLevel, signRepodata, gpgKey, remaining[1:])
	case "add":
		return runAdd(ctx, backendType, repoRoot, s3Endpoint, logLevel, signRPMs, gpgKey, remaining[1:])
	case "remove":
		return runRemove(ctx, backendType, repoRoot, s3Endpoint, logLevel, remaining[1:])
	case "check":
		return runCheck(ctx, backendType, repoRoot, s3Endpoint, logLevel, outputFormat, remaining[1:])
	default:
		return fmt.Errorf("unknown command %q", remaining[0])
	}
}

func runInit(ctx context.Context, backendType, repoRoot, s3Endpoint, logLevel string, signRepodata bool, gpgKey string, args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var checksum string
	var force bool
	fs.StringVar(&checksum, "checksum", "sha256", "checksum algorithm (sha256 or sha512)")
	fs.BoolVar(&force, "force", false, "overwrite existing repomd.xml")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if repoRoot == "" {
		return fmt.Errorf("--repo-root is required")
	}
	b, err := buildBackend(ctx, backendType, repoRoot, s3Endpoint)
	if err != nil {
		return err
	}
	r, err := newRepoWithLogger(b, logLevel)
	if err != nil {
		return err
	}
	if err := r.InitRepo(ctx, checksum, force, signRepodata, gpgKey); err != nil {
		return err
	}
	fmt.Fprintf(os.Stdout, "initialized repo at %s (checksum: %s)\n", repoRoot, checksum)
	return nil
}

func runAdd(ctx context.Context, backendType, repoRoot, s3Endpoint, logLevel string, signRPMs bool, gpgKey string, args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var replaceExisting bool
	var dryRun bool
	var duplicatePolicy string
	var allowUnknown bool
	var destPrefix string
	fs.BoolVar(&replaceExisting, "replace-existing", false, "replace packages with the same NEVRA")
	fs.BoolVar(&dryRun, "dry-run", false, "show planned changes without writing")
	fs.StringVar(&duplicatePolicy, "on-duplicate", "error", "behavior when NEVRA exists (error|replace)")
	fs.BoolVar(&allowUnknown, "allow-unknown", true, "preserve unknown metadata types instead of error")
	fs.StringVar(&destPrefix, "dest-prefix", "", "destination prefix for RPMs inside repo (default: basename in root)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if repoRoot == "" {
		return fmt.Errorf("--repo-root is required")
	}
	rpmPaths := fs.Args()
	if len(rpmPaths) == 0 {
		return fmt.Errorf("add requires at least one RPM path")
	}
	b, err := buildBackend(ctx, backendType, repoRoot, s3Endpoint)
	if err != nil {
		return err
	}
	r, err := newRepoWithLogger(b, logLevel)
	if err != nil {
		return err
	}
	if duplicatePolicy == "replace" {
		replaceExisting = true
	} else if duplicatePolicy != "error" {
		return fmt.Errorf("invalid --on-duplicate %q", duplicatePolicy)
	}
	r.AllowUnknown = allowUnknown
	r.DestPrefix = destPrefix
	if err := r.AddRPMs(ctx, rpmPaths, replaceExisting, dryRun, signRPMs, gpgKey); err != nil {
		return err
	}
	if dryRun {
		for _, p := range rpmPaths {
			fmt.Fprintf(os.Stdout, "would add %s\n", p)
		}
	} else {
		for _, p := range rpmPaths {
			fmt.Fprintf(os.Stdout, "added %s\n", p)
		}
	}
	return nil
}

func runRemove(ctx context.Context, backendType, repoRoot, s3Endpoint, logLevel string, args []string) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var deleteFiles bool
	var byNEVRA bool
	var dryRun bool
	var allowUnknown bool
	fs.BoolVar(&deleteFiles, "delete-files", false, "delete matching RPM files")
	fs.BoolVar(&byNEVRA, "by-nevra", false, "treat identifiers as NEVRA instead of filenames")
	fs.BoolVar(&dryRun, "dry-run", false, "show planned changes without writing")
	fs.BoolVar(&allowUnknown, "allow-unknown", true, "preserve unknown metadata types instead of error")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if repoRoot == "" {
		return fmt.Errorf("--repo-root is required")
	}
	ids := fs.Args()
	if len(ids) == 0 {
		return fmt.Errorf("remove requires at least one identifier")
	}
	b, err := buildBackend(ctx, backendType, repoRoot, s3Endpoint)
	if err != nil {
		return err
	}
	r, err := newRepoWithLogger(b, logLevel)
	if err != nil {
		return err
	}
	r.AllowUnknown = allowUnknown
	if err := r.RemoveRPMs(ctx, ids, byNEVRA, deleteFiles, dryRun); err != nil {
		return err
	}
	if dryRun {
		for _, id := range ids {
			fmt.Fprintf(os.Stdout, "would remove %s\n", id)
		}
	} else {
		for _, id := range ids {
			fmt.Fprintf(os.Stdout, "removed %s\n", id)
		}
	}
	return nil
}

func runCheck(ctx context.Context, backendType, repoRoot, s3Endpoint, logLevel, outputFormat string, args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if repoRoot == "" {
		return fmt.Errorf("--repo-root is required")
	}
	b, err := buildBackend(ctx, backendType, repoRoot, s3Endpoint)
	if err != nil {
		return err
	}
	r, err := newRepoWithLogger(b, logLevel)
	if err != nil {
		return err
	}
	result := r.CheckDetailed(ctx)
	if result.Err != nil {
		return result.Err
	}
	switch outputFormat {
	case "text":
		for _, w := range result.Warnings {
			fmt.Fprintf(os.Stdout, "warn: %s\n", w)
		}
		fmt.Fprintf(os.Stdout, "repo ok at %s\n", repoRoot)
	case "json":
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
	default:
		return fmt.Errorf("unknown output format %q", outputFormat)
	}
	return nil
}

func buildBackend(ctx context.Context, backendType, repoRoot, s3Endpoint string) (backend.Backend, error) {
	switch backendType {
	case "fs":
		return backend.NewFSBackend(repoRoot), nil
	case "s3":
		return backend.NewS3Backend(ctx, repoRoot, s3Endpoint)
	default:
		return nil, fmt.Errorf("backend %q not implemented", backendType)
	}
}

func newRepoWithLogger(b backend.Backend, level string) (*repo.Repo, error) {
	r := repo.New(b)
	switch strings.ToLower(level) {
	case "error":
		r.WithLogger(io.Discard)
	case "info", "debug":
		// future: emit debug logs.
	default:
		return nil, fmt.Errorf("unknown log level %q", level)
	}
	return r, nil
}
