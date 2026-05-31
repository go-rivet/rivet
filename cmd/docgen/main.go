package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type FunctionRow struct {
	Name      string
	Signature string
}

func main() {
	fset := token.NewFileSet()
	baseDir := "."

	subDirs, err := os.ReadDir(baseDir)
	if err != nil {
		panic(err)
	}

	outputDir := "../../build/doc/templater"
	_ = os.MkdirAll(outputDir, 0755)

	for _, entry := range subDirs {
		if !entry.IsDir() {
			continue
		}

		folderName := entry.Name() // e.g., "task" or "sprig"
		folderPath := filepath.Join(baseDir, folderName)

		caser := cases.Title(language.English)
		cleanFolderName := caser.String(strings.ReplaceAll(folderName, "-", " "))
		pageTitle := fmt.Sprintf("%s Functions", cleanFolderName)

		// 1. Read files inside the directory
		files, err := os.ReadDir(folderPath)
		if err != nil {
			continue
		}

		// 2. Parse Go files into an independent slice (replaces ast.Package completely)
		var parsedFiles []*ast.File
		for _, fileEntry := range files {
			if fileEntry.IsDir() || !strings.HasSuffix(fileEntry.Name(), ".go") || strings.HasSuffix(fileEntry.Name(), "_test.go") {
				continue
			}

			filePath := filepath.Join(folderPath, fileEntry.Name())
			fileNode, err := parser.ParseFile(fset, filePath, nil, parser.ParseComments)
			if err != nil {
				continue
			}
			parsedFiles = append(parsedFiles, fileNode)
		}

		if len(parsedFiles) == 0 {
			continue
		}

		// Memory Buffers to organize structure before writing strings
		currentGroup := "General"
		groupOrder := []string{}
		groupMap := make(map[string][]FunctionRow)

		// 3. Iterate over our modern parsedFiles slice
		for _, fileNode := range parsedFiles {
			ast.Inspect(fileNode, func(n ast.Node) bool {
				kv, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}

				// Identify and extract group headers
				kvLine := fset.Position(kv.Pos()).Line
				for _, cgo := range fileNode.Comments {
					for _, comment := range cgo.List {
						commentLine := fset.Position(comment.Pos()).Line

						if commentLine == kvLine-1 {
							commentText := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
							if strings.HasPrefix(commentText, "---") && strings.HasSuffix(commentText, "---") {
								currentGroup = strings.Trim(commentText, "- ")
							}
						}
					}
				}

				// Extract the actual template map row key
				lit, ok := kv.Key.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				funcName := strings.Trim(lit.Value, `"`)

				// Track unique group names sequentially to prevent duplication drops
				if _, checked := groupMap[currentGroup]; !checked {
					groupOrder = append(groupOrder, currentGroup)
					groupMap[currentGroup] = []FunctionRow{}
				}

				// Pass the slice of sibling files to resolve function declarations
				sig := parseSignatureStatically(kv.Value, parsedFiles)
				groupMap[currentGroup] = append(groupMap[currentGroup], FunctionRow{
					Name:      funcName,
					Signature: sig,
				})
				return true
			})
		}

		// Only compile and generate markdown files if active content was found
		if len(groupOrder) > 0 {
			// Sort Group Headers Alphabetically
			slices.Sort(groupOrder)

			var buf bytes.Buffer
			fmt.Fprintf(&buf, "---\ntitle: %s\nsidebar_label: %s\n---\n\n", pageTitle, cleanFolderName)
			fmt.Fprintf(&buf, "# %s\n\n", pageTitle)

			for _, group := range groupOrder {
				rows := groupMap[group]
				if len(rows) == 0 {
					continue
				}

				// Sort functions within this group alphabetically by name
				slices.SortFunc(rows, func(a, b FunctionRow) int {
					return strings.Compare(a.Name, b.Name)
				})

				fmt.Fprintf(&buf, "\n### %s\n\n", group)
				buf.WriteString("| Function | Go Signature |\n| :--- | :--- |\n")
				for _, row := range rows {
					fmt.Fprintf(&buf, "| **`%s`** | `%s` |\n", row.Name, row.Signature)
				}
			}

			outputFile := filepath.Join(outputDir, fmt.Sprintf("%s.md", folderName))
			err = os.WriteFile(outputFile, buf.Bytes(), 0644)
			if err != nil {
				panic(err)
			}
			fmt.Printf("Generated sorted nested sub-entry: %s\n", outputFile)
		}
	}
}

// parseSignatureStatically resolves parameter text across the parsed file tree slice
func parseSignatureStatically(expr ast.Expr, folderFiles []*ast.File) string {
	switch val := expr.(type) {
	case *ast.FuncLit:
		return formatFuncFields(val.Type)
	case *ast.Ident:
		// Modern scanning across our explicit file slice instead of ast.Package map
		for _, fileNode := range folderFiles {
			for _, decl := range fileNode.Decls {
				if fDecl, ok := decl.(*ast.FuncDecl); ok && fDecl.Name.Name == val.Name {
					return formatFuncFields(fDecl.Type)
				}
			}
		}
		return "()"
	case *ast.SelectorExpr:
		if xIdent, ok := val.X.(*ast.Ident); ok {
			return fmt.Sprintf("via %s.%s", xIdent.Name, val.Sel.Name)
		}
	}
	return "()"
}

// formatFuncFields translates inputs and outputs into clean documentation signatures
func formatFuncFields(fType *ast.FuncType) string {
	var inputs []string
	if fType.Params != nil {
		for i, field := range fType.Params.List {
			typeStr := getASTTypeString(field.Type)
			nameStr := fmt.Sprintf("arg%d", i+1)
			if len(field.Names) > 0 {
				nameStr = field.Names[0].Name
			}
			inputs = append(inputs, fmt.Sprintf("%s %s", nameStr, typeStr))
		}
	}

	var outputs []string
	if fType.Results != nil {
		for _, field := range fType.Results.List {
			outputs = append(outputs, getASTTypeString(field.Type))
		}
	}

	inStr := strings.Join(inputs, ", ")
	outStr := strings.Join(outputs, ", ")

	inStr = strings.ReplaceAll(inStr, "interface{}", "any")
	outStr = strings.ReplaceAll(outStr, "interface{}", "any")

	if len(outputs) > 1 {
		return fmt.Sprintf("(%s) (%s)", inStr, outStr)
	} else if len(outputs) == 1 {
		return fmt.Sprintf("(%s) %s", inStr, outStr)
	}
	return fmt.Sprintf("(%s)", inStr)
}

// getASTTypeString formats AST expression structures back into native source code text
func getASTTypeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return id.Name + "." + t.Sel.Name
		}
	case *ast.Ellipsis:
		return "..." + getASTTypeString(t.Elt)
	case *ast.ArrayType:
		return "[]" + getASTTypeString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", getASTTypeString(t.Key), getASTTypeString(t.Value))
	case *ast.InterfaceType:
		return "any"
	}
	return "any"
}
