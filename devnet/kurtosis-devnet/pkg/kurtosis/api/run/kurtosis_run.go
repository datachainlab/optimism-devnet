package run

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/datachainlab/optimism-devnet/devnet/kurtosis-devnet/pkg/kurtosis/api/enclave"
	"github.com/datachainlab/optimism-devnet/devnet/kurtosis-devnet/pkg/kurtosis/api/interfaces"
	"github.com/datachainlab/optimism-devnet/devnet/kurtosis-devnet/pkg/kurtosis/api/wrappers"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/starlark_run_config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

type KurtosisRunner struct {
	dryRun      bool
	enclave     string
	baseDir     string
	kurtosisCtx interfaces.KurtosisContextInterface
	runHandlers []MessageHandler
	tracer      trace.Tracer
}

type KurtosisRunnerOptions func(*KurtosisRunner)

func WithKurtosisRunnerDryRun(dryRun bool) KurtosisRunnerOptions {
	return func(r *KurtosisRunner) {
		r.dryRun = dryRun
	}
}

func WithKurtosisRunnerEnclave(enclave string) KurtosisRunnerOptions {
	return func(r *KurtosisRunner) {
		r.enclave = enclave
	}
}

func WithKurtosisRunnerBaseDir(baseDir string) KurtosisRunnerOptions {
	return func(r *KurtosisRunner) {
		r.baseDir = baseDir
	}
}

func WithKurtosisRunnerKurtosisContext(kurtosisCtx interfaces.KurtosisContextInterface) KurtosisRunnerOptions {
	return func(r *KurtosisRunner) {
		r.kurtosisCtx = kurtosisCtx
	}
}

func WithKurtosisRunnerRunHandlers(runHandlers ...MessageHandler) KurtosisRunnerOptions {
	return func(r *KurtosisRunner) {
		r.runHandlers = runHandlers
	}
}

func NewKurtosisRunner(opts ...KurtosisRunnerOptions) (*KurtosisRunner, error) {
	r := &KurtosisRunner{
		tracer: otel.Tracer("kurtosis-run"),
	}
	for _, opt := range opts {
		opt(r)
	}

	if r.kurtosisCtx == nil {
		var err error
		r.kurtosisCtx, err = wrappers.GetDefaultKurtosisContext()
		if err != nil {
			return nil, fmt.Errorf("failed to create Kurtosis context: %w", err)
		}
	}
	return r, nil
}

// isLocalPackage checks if the package path is a local path (not a remote GitHub reference)
func isLocalPackage(packageName string) bool {
	return strings.HasPrefix(packageName, "./") || strings.HasPrefix(packageName, "/") || strings.HasPrefix(packageName, "../")
}

// runViaCLI runs the kurtosis package using the CLI command
func (r *KurtosisRunner) runViaCLI(ctx context.Context, packageName string, args io.Reader) error {
	// Resolve the package path
	var packagePath string
	if filepath.IsAbs(packageName) {
		packagePath = packageName
	} else {
		packagePath = filepath.Join(r.baseDir, packageName)
	}

	// Create a temp file for args if provided
	var argsFile string
	if args != nil {
		argsBytes, err := io.ReadAll(args)
		if err != nil {
			return fmt.Errorf("failed to read args: %w", err)
		}
		tmpFile, err := os.CreateTemp("", "kurtosis-args-*.yaml")
		if err != nil {
			return fmt.Errorf("failed to create temp file for args: %w", err)
		}
		defer os.Remove(tmpFile.Name())
		if _, err := tmpFile.Write(argsBytes); err != nil {
			tmpFile.Close()
			return fmt.Errorf("failed to write args to temp file: %w", err)
		}
		tmpFile.Close()
		argsFile = tmpFile.Name()
	}

	// Build the kurtosis command
	cmdArgs := []string{"run", packagePath, "--enclave", r.enclave}
	if argsFile != "" {
		cmdArgs = append(cmdArgs, "--args-file", argsFile)
	}

	cmd := exec.CommandContext(ctx, "kurtosis", cmdArgs...)
	cmd.Dir = r.baseDir

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start kurtosis CLI: %w", err)
	}

	// Stream stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println(scanner.Text())
		}
	}()

	// Stream stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Fprintln(os.Stderr, scanner.Text())
		}
	}()

	// Wait for the command to complete
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("kurtosis CLI failed: %w", err)
	}

	return nil
}

func (r *KurtosisRunner) Run(ctx context.Context, packageName string, args io.Reader) error {
	ctx, span := r.tracer.Start(ctx, fmt.Sprintf("run package %s", packageName))
	defer span.End()

	if r.dryRun {
		fmt.Printf("Dry run mode enabled, would run kurtosis package %s in enclave %s\n",
			packageName, r.enclave)
		if args != nil {
			fmt.Println("\nWith arguments:")
			if _, err := io.Copy(os.Stdout, args); err != nil {
				return fmt.Errorf("failed to dump args: %w", err)
			}
			fmt.Println()
		}
		return nil
	}

	// Use CLI for local packages to work around SDK compatibility issues with Kurtosis 1.18+
	if isLocalPackage(packageName) {
		return r.runViaCLI(ctx, packageName, args)
	}

	mgr, err := enclave.NewKurtosisEnclaveManager(
		enclave.WithKurtosisContext(r.kurtosisCtx),
	)
	if err != nil {
		return fmt.Errorf("failed to create Kurtosis enclave manager: %w", err)
	}
	// Try to get existing enclave first
	enclaveCtx, err := mgr.GetEnclave(ctx, r.enclave)
	if err != nil {
		return fmt.Errorf("failed to get enclave: %w", err)
	}

	// Set up run config with args if provided
	serializedParams := "{}"
	if args != nil {
		argsBytes, err := io.ReadAll(args)
		if err != nil {
			return fmt.Errorf("failed to read args: %w", err)
		}
		serializedParams = string(argsBytes)
	}

	runConfig := &starlark_run_config.StarlarkRunConfig{
		SerializedParams: serializedParams,
	}

	stream, _, err := enclaveCtx.RunStarlarkPackage(ctx, packageName, runConfig)
	if err != nil {
		return fmt.Errorf("failed to run Kurtosis package: %w", err)
	}

	// Set up message handlers
	var isRunSuccessful bool
	runFinishedHandler := makeRunFinishedHandler(&isRunSuccessful)

	// Combine custom handlers with default handler and run finished handler
	handler := AllHandlers(append(r.runHandlers, newDefaultHandler(), runFinishedHandler)...)

	// Process the output stream
	for responseLine := range stream {
		if _, err := handler.Handle(ctx, responseLine); err != nil {
			return err
		}
	}

	if !isRunSuccessful {
		return errors.New(printRed("kurtosis package execution failed"))
	}

	return nil
}

func (r *KurtosisRunner) RunScript(ctx context.Context, script string) error {
	if r.dryRun {
		fmt.Printf("Dry run mode enabled, would run following script in enclave %s\n%s\n",
			r.enclave, script)
		return nil
	}

	enclaveCtx, err := r.kurtosisCtx.GetEnclave(ctx, r.enclave)
	if err != nil {
		return fmt.Errorf("failed to get enclave: %w", err)
	}

	return enclaveCtx.RunStarlarkScript(ctx, script, &starlark_run_config.StarlarkRunConfig{
		SerializedParams: "{}",
	})
}
