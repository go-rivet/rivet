package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
)

func genCliDoc() {
	outFile := "../../build/doc/cli.md"
	filePath := "config.go"

	_ = os.MkdirAll("../../build/doc", 0755)
	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		return
	}
	defer func() { _ = f.Close() }()
	oldStdout := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = oldStdout }()

	fset := token.NewFileSet()

	node, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
	if err != nil {
		fmt.Printf("Error parsing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("# CLI")
	fmt.Println()
	fmt.Println("## Usage")
	fmt.Println()
	fmt.Println("```sh")
	fmt.Println("rivet [OPTIONS] [tasks...] [-- ARGS]")
	fmt.Println("```")
	fmt.Println()
	fmt.Println("## Options")

	ast.Inspect(node, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || ts.Name.Name != "Config" {
			return true
		}

		structType, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}

		printMarkdownDocs(structType)
		return false
	})

	fmt.Println()
	fmt.Println("## Examples")
	fmt.Println()
	fmt.Println("### version")
	fmt.Println()
	fmt.Println("```bash")
	fmt.Println("rivet --version")
	fmt.Println()
	fmt.Println("# Expected output:")
	fmt.Println("Rivet v0.2.0")
	fmt.Println("Commit: ffc9bc74")
	fmt.Println("Built:  2026-05-17T12:32:58Z")
	fmt.Println("```")
}

type SectionMetaData struct {
	HasAnyShort bool
	HasAnyEnv   bool
	Fields      []*ast.Field
}

func printMarkdownDocs(structType *ast.StructType) {
	var sections []SectionMetaData
	var currentSection *SectionMetaData

	for _, field := range structType.Fields.List {
		if len(field.Names) == 0 || field.Tag == nil {
			continue
		}

		tagStr := strings.Trim(field.Tag.Value, "`")
		tags := reflect.StructTag(tagStr)

		docText, hasDoc := tags.Lookup("doc")
		if !hasDoc || docText == "" {
			continue
		}

		if field.Doc != nil || currentSection == nil {
			sections = append(sections, SectionMetaData{})
			currentSection = &sections[len(sections)-1]
		}

		currentSection.Fields = append(currentSection.Fields, field)

		if tags.Get("sflag") != "" {
			currentSection.HasAnyShort = true
		}
		if tags.Get("noenv") != "true" {
			currentSection.HasAnyEnv = true
		}
	}

	for _, sec := range sections {
		if len(sec.Fields) == 0 {
			continue
		}

		if sec.Fields[0].Doc != nil {
			sectionTitle := strings.TrimSpace(sec.Fields[0].Doc.Text())
			fmt.Printf("\n### %s\n\n", sectionTitle)
		}

		header := "| Option"
		divider := "| :---"
		if sec.HasAnyShort {
			header += " | Short"
			divider += " | :---"
		}
		if sec.HasAnyEnv {
			header += " | Env"
			divider += " | :---"
		}
		header += " | Description |"
		divider += " | :--- |"

		fmt.Println(header)
		fmt.Println(divider)

		for _, field := range sec.Fields {
			tagStr := strings.Trim(field.Tag.Value, "`")
			tags := reflect.StructTag(tagStr)
			docText := tags.Get("doc")
			fieldName := field.Names[0].Name

			flagName := tags.Get("flag")
			if flagName == "" {
				flagName = toKebabCase(fieldName)
			}

			fieldTypeStr := fmt.Sprintf("%v", field.Type)
			if fieldTypeStr != "bool" && !strings.Contains(flagName, "<") {
				placeholder := "<value>"
				if fieldTypeStr == "time.Duration" {
					placeholder = "<duration>"
				} else if fieldTypeStr == "string" && (flagName == "sort" || flagName == "log") {
					placeholder = "<format>"
				}
				if flagName == "verbose" {
					placeholder = "<level>"
				}
				flagName = fmt.Sprintf("%s %s", flagName, placeholder)
			}

			// FIX: Wrap the option in a no-wrap span container
			optionCell := fmt.Sprintf("<span style=\"white-space: nowrap;\">`--%s`</span>", flagName)
			row := fmt.Sprintf("| %s", optionCell)

			if sec.HasAnyShort {
				shortFlag := tags.Get("sflag")
				if shortFlag != "" {
					row += fmt.Sprintf(" | `-%s`", shortFlag)
				} else {
					row += " | "
				}
			}

			if sec.HasAnyEnv {
				envVar := ""
				if tags.Get("noenv") != "true" {
					envVar = fmt.Sprintf("<span style=\"white-space: nowrap;\">`RIVET_%s`</span>", toUpperSnakeCase(fieldName))
				}
				row += fmt.Sprintf(" | %s", envVar)
			}

			row += fmt.Sprintf(" | %s |", formatDoc(docText))
			fmt.Println(row)
		}
	}
}

func toKebabCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func toUpperSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToUpper(result.String())
}

func formatDoc(doc string) string {
	return strings.ReplaceAll(doc, "\n", "<br>")
}
