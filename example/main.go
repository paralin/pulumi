package main

/*
 * DIRECT PULUMI ENGINE EXAMPLE
 *
 * This example demonstrates how to use the Pulumi engine directly without
 * using the Automation API or running the CLI as a subprocess.
 *
 * The Automation API (sdk/v3/go/auto) provides a convenient wrapper around
 * the CLI, executing it as a subprocess and capturing its output. This example
 * instead uses the core Pulumi engine packages directly from pkg/, which is
 * the same code that the CLI uses internally.
 *
 * Key differences from the Automation API approach:
 * 1. Direct access to the engine and backend APIs
 * 2. No subprocess execution of the CLI binary
 * 3. Direct event handling from the engine
 * 4. More control over the deployment lifecycle
 */

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pulumi/pulumi/pkg/v3/backend"
	"github.com/pulumi/pulumi/pkg/v3/backend/display"
	"github.com/pulumi/pulumi/pkg/v3/backend/diy"
	"github.com/pulumi/pulumi/pkg/v3/engine"
	"github.com/pulumi/pulumi/pkg/v3/resource/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag/colors"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	// pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

func main() {
	// Get the absolute path to the project directory
	projectDir, err := filepath.Abs("./project")
	if err != nil {
		log.Fatalf("Failed to get absolute path: %v", err)
	}

	fmt.Printf("Using Pulumi project at: %s\n", projectDir)

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("Failed to get working directory: %v", err)
	}

	// Create .pulumi directory if it doesn't exist
	// pulumiDir := filepath.Join(cwd, ".pulumi")
	//if err := os.MkdirAll(pulumiDir, 0755); err != nil {
	//	log.Fatalf("Failed to create .pulumi directory: %v", err)
	//}
	pulumiDir := cwd

	// Set up environment variables
	os.Setenv("PULUMI_CONFIG_PASSPHRASE", "example")

	projectPath, err := workspace.DetectProjectPathFrom(projectDir)
	if err != nil {
		log.Fatalf("Failed to find project: %v", err)
	}

	// Load project settings
	projSettings, err := workspace.LoadProject(projectPath)
	if err != nil {
		log.Fatalf("Failed to load project: %v", err)
	}
	fmt.Printf("Project name: %s\n", projSettings.Name)
	if projSettings.Description != nil {
		fmt.Printf("Project description: %s\n", *projSettings.Description)
	}

	// Create a diagnostic sink for logging
	diagSink := diag.DefaultSink(os.Stdout, os.Stderr, diag.FormatOptions{
		Color: colors.Always,
	})

	// Construct the file URL for the DIY backend
	fileURL := fmt.Sprintf("file://%s", pulumiDir)

	// Initialize a local DIY localBackend
	localBackend, err := diy.New(ctx, diagSink, fileURL, projSettings)
	if err != nil {
		log.Fatalf("Failed to create backend: %v", err)
	}

	// Get the stack name, defaulting to "dev"
	stackName := "dev"
	if len(os.Args) > 1 {
		stackName = os.Args[1]
	}

	stackRef, err := localBackend.ParseStackReference(stackName)
	if err != nil {
		log.Fatalf("Failed to parse stack reference: %v", err)
	}
	fmt.Printf("Using stack: %s\n", stackRef)

	// Get or create the localStack
	localStack, err := localBackend.GetStack(ctx, stackRef)
	if localStack == nil || err != nil {
		// For CreateStack, pass the project root directory and nil for initialState and opts
		localStack, err = localBackend.CreateStack(ctx, stackRef, projectDir, nil, nil)
		if err != nil {
			log.Fatalf("Failed to create stack: %v", err)
		}
	}

	// Set up event channels
	eventSink := make(chan engine.Event)
	done := make(chan bool)

	// Set up the progress display
	displayOpts := display.Options{
		Color:                colors.Always,
		Debug:                false,
		ShowConfig:           true,
		ShowReplacementSteps: true,
		ShowSameResources:    false,
		ShowReads:            true,
		SuppressOutputs:      false,
		IsInteractive:        true,
		Type:                 display.DisplayProgress,
		JSONDisplay:          false,
		ShowSecrets:          false,
		SuppressPermalink:    true,
	}

	// Start the progress display
	go display.ShowProgressEvents(
		"pulumi",
		apitype.UpdateUpdate,
		stackRef.Name(),
		tokens.PackageName(projSettings.Name.String()),
		"", // no permalink for local backend
		eventSink,
		done,
		displayOpts,
		false, // not a preview
	)

	// Create the update options
	updateOpts := backend.UpdateOperation{
		Proj: projSettings,
		Root: projectDir,
		Opts: backend.UpdateOptions{
			Display:     displayOpts,
			AutoApprove: true,
		},
		SecretsProvider: stack.DefaultSecretsProvider,
		Scopes:          backend.CancellationScopes,
		M:               &backend.UpdateMetadata{Message: "example update"},
	}

	// Run a preview to see what would change
	fmt.Println("Running preview...")
	plan, resourceChanges, err := localStack.Preview(ctx, updateOpts, eventSink)
	if err != nil {
		log.Fatalf("Preview failed: %v", err)
	}
	_ = plan

	// Print the preview summary
	fmt.Println("Preview summary:")
	for k, v := range resourceChanges {
		if v > 0 {
			fmt.Printf("  %s: %d\n", k, v)
		}
	}

	// Run the update
	fmt.Println("Running update...")
	resourceChanges, err = localStack.Update(ctx, updateOpts)
	if err != nil {
		log.Fatalf("Update failed: %v", err)
	}

	// Print the update results
	fmt.Println("Update results:")
	for k, v := range resourceChanges {
		if v > 0 {
			fmt.Printf("  %s: %d\n", k, v)
		}
	}

	// Get and print the outputs
	/*
		outputs, err := stack.Outputs(ctx)
		if err != nil {
			log.Fatalf("Failed to get outputs: %v", err)
		}

		fmt.Println("Stack outputs:")
		for k, v := range outputs {
			secretStr := ""
			if v.Secret {
				secretStr = " (secret)"
			}
			fmt.Printf("  %s = %v%s\n", k, v.Value, secretStr)
		}
	*/
}
