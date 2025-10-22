// generate-unified-sdk creates a unified SDK that combines all service SDKs
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

type ServiceSDK struct {
	PackageName string // e.g., "accountv1"
	ImportPath  string // e.g., "github.com/plaenen/eventstore/examples/pb/account/v1"
	SDKType     string // e.g., "AccountSDK"
	FieldName   string // e.g., "Account"
}

type UnifiedSDKData struct {
	PackageName      string
	Services         []ServiceSDK
	EventsourcingPkg string
}

func main() {
	// Define CLI flags
	var (
		pbDir           string
		outputFile      string
		modulePath      string
		outputPkg       string
		eventsourcingPkg string
	)

	flag.StringVar(&pbDir, "pb-dir", "", "Directory containing protobuf generated files (required)")
	flag.StringVar(&outputFile, "output", "", "Output file path (required)")
	flag.StringVar(&modulePath, "module", "", "Go module path (auto-detected if not provided)")
	flag.StringVar(&outputPkg, "package", "sdk", "Package name for generated SDK")
	flag.StringVar(&eventsourcingPkg, "eventsourcing", "github.com/plaenen/eventstore/pkg/eventsourcing", "Import path for eventsourcing package")
	flag.Parse()

	// Validate required flags
	if pbDir == "" || outputFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -pb-dir <dir> -output <file> [options]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nRequired flags:\n")
		fmt.Fprintf(os.Stderr, "  -pb-dir string\n")
		fmt.Fprintf(os.Stderr, "        Directory containing protobuf generated files\n")
		fmt.Fprintf(os.Stderr, "  -output string\n")
		fmt.Fprintf(os.Stderr, "        Output file path\n")
		fmt.Fprintf(os.Stderr, "\nOptional flags:\n")
		fmt.Fprintf(os.Stderr, "  -module string\n")
		fmt.Fprintf(os.Stderr, "        Go module path (auto-detected from go.mod if not provided)\n")
		fmt.Fprintf(os.Stderr, "  -package string\n")
		fmt.Fprintf(os.Stderr, "        Package name for generated SDK (default \"sdk\")\n")
		fmt.Fprintf(os.Stderr, "  -eventsourcing string\n")
		fmt.Fprintf(os.Stderr, "        Import path for eventsourcing package (default \"github.com/plaenen/eventstore/pkg/eventsourcing\")\n")
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -pb-dir ./examples/pb -output ./examples/sdk/unified.go\n", os.Args[0])
		os.Exit(1)
	}

	// Auto-detect module path if not provided
	if modulePath == "" {
		detected, err := detectModulePath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to auto-detect module path: %v\n", err)
			fmt.Fprintf(os.Stderr, "Please specify -module flag explicitly\n")
			os.Exit(1)
		}
		modulePath = detected
		fmt.Printf("Auto-detected module path: %s\n", modulePath)
	}

	services, err := discoverServices(pbDir, modulePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering services: %v\n", err)
		os.Exit(1)
	}

	if len(services) == 0 {
		fmt.Fprintf(os.Stderr, "No services found in %s\n", pbDir)
		os.Exit(1)
	}

	fmt.Printf("Found %d services:\n", len(services))
	for _, svc := range services {
		fmt.Printf("  - %s.%s\n", svc.PackageName, svc.SDKType)
	}

	if err := generateUnifiedSDK(services, outputFile, outputPkg, eventsourcingPkg); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating unified SDK: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Generated unified SDK: %s\n", outputFile)
}

// detectModulePath auto-detects the Go module path from go.mod
func detectModulePath() (string, error) {
	cmd := exec.Command("go", "list", "-m")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run 'go list -m': %w", err)
	}
	modulePath := strings.TrimSpace(string(output))
	if modulePath == "" {
		return "", fmt.Errorf("empty module path returned")
	}
	return modulePath, nil
}

// discoverServices scans the pb directory for generated SDK files
func discoverServices(pbDir string, modulePath string) ([]ServiceSDK, error) {
	var services []ServiceSDK

	err := filepath.Walk(pbDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Look for *_sdk.es.pb.go files
		if !info.IsDir() && strings.HasSuffix(info.Name(), "_sdk.es.pb.go") {
			sdks, err := parseSDKFile(path, pbDir, modulePath)
			if err != nil {
				return fmt.Errorf("failed to parse %s: %w", path, err)
			}
			services = append(services, sdks...)
		}

		return nil
	})

	return services, err
}

// parseSDKFile parses a *_sdk.pb.go file to extract SDK information
func parseSDKFile(filePath, pbDir, modulePath string) ([]ServiceSDK, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	var sdks []ServiceSDK
	packageName := node.Name.Name

	// Determine import path by combining module path with relative path
	relPath, err := filepath.Rel(pbDir, filepath.Dir(filePath))
	if err != nil {
		return nil, err
	}

	// Build import path: modulePath/pbDir/relPath
	// First, get the relative path from module root to pbDir
	absModuleRoot, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	absPbDir, err := filepath.Abs(pbDir)
	if err != nil {
		return nil, err
	}
	pbDirFromRoot, err := filepath.Rel(absModuleRoot, absPbDir)
	if err != nil {
		return nil, err
	}

	// Combine module path + pb directory path + relative path
	importPath := modulePath
	if pbDirFromRoot != "." {
		importPath = filepath.Join(modulePath, pbDirFromRoot)
	}
	if relPath != "." {
		importPath = filepath.Join(importPath, relPath)
	}
	// Convert to forward slashes for Go import path
	importPath = filepath.ToSlash(importPath)

	// Find all SDK types (types ending with "SDK")
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			typeName := typeSpec.Name.Name
			if strings.HasSuffix(typeName, "SDK") {
				// Extract service name (e.g., "AccountSDK" -> "Account")
				serviceName := strings.TrimSuffix(typeName, "SDK")

				sdks = append(sdks, ServiceSDK{
					PackageName: packageName,
					ImportPath:  importPath,
					SDKType:     typeName,
					FieldName:   serviceName,
				})
			}
		}
	}

	return sdks, nil
}

// generateUnifiedSDK generates the unified SDK Go file
func generateUnifiedSDK(services []ServiceSDK, outputFile string, packageName string, eventsourcingPkg string) error {
	// Create output directory if it doesn't exist
	outputDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Prepare template data
	data := UnifiedSDKData{
		PackageName:      packageName,
		Services:         services,
		EventsourcingPkg: eventsourcingPkg,
	}

	// Execute template
	tmpl, err := template.New("unified_sdk").Parse(unifiedSDKTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template: %w", err)
	}

	f, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	return nil
}

const unifiedSDKTemplate = `// Code generated by generate-unified-sdk. DO NOT EDIT.

package {{.PackageName}}

import (
{{range .Services}}	{{.PackageName}} "{{.ImportPath}}"
{{end}}	"{{.EventsourcingPkg}}"
)

// SDK provides a unified interface to all services in the application.
// It combines all service SDKs into a single client that only requires one transport.
//
// Example usage:
//
//	transport, _ := nats.NewTransport(&nats.TransportConfig{...})
//	sdk := NewSDK(transport)
//	defer sdk.Close()
//
//	// Use any service
{{range .Services}}//	sdk.{{.FieldName}}.OpenAccount(ctx, cmd)
{{end -}}
type SDK struct {
{{range .Services}}	// {{.FieldName}} provides access to {{.FieldName}} service operations
	{{.FieldName}} *{{.PackageName}}.{{.SDKType}}
{{end}}
	transport eventsourcing.Transport
}

// NewSDK creates a new unified SDK that combines all services.
// It only requires a single transport - all service SDKs are created automatically.
func NewSDK(transport eventsourcing.Transport) *SDK {
	return &SDK{
{{range .Services}}		{{.FieldName}}: {{.PackageName}}.New{{.SDKType}}(transport),
{{end}}		transport: transport,
	}
}

// Transport returns the underlying transport used by all service SDKs.
// This can be useful for advanced use cases or debugging.
func (s *SDK) Transport() eventsourcing.Transport {
	return s.transport
}

// Close closes the underlying transport connection.
// This will close the connection for all service SDKs.
func (s *SDK) Close() error {
	return s.transport.Close()
}
`
