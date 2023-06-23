package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"os"
	"path/filepath"
)
import "golang.org/x/tools/go/loader" //nolint:staticcheck

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	conf := loader.Config{ParserMode: parser.ParseComments}
	conf.Build = &build.Default
	conf.Build.CgoEnabled = false
	projectPath := os.Args[1]
	conf.Build.Dir = filepath.Join(cwd, projectPath)
	conf.Import(projectPath)
	prog, err := conf.Load()
	if err != nil {
		fmt.Println(err)
	}

	for _, pkg := range prog.AllPackages {

		fmt.Printf("Package path %q\n", pkg.Pkg.Path())
		for _, file := range pkg.Files {
			_ = file
			fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
			ast.Inspect(file, func(n ast.Node) bool {
				if funDeclNode, ok := n.(*ast.FuncDecl); ok {
					fmt.Println("FuncDecl:" + file.Name.Name + "." + funDeclNode.Name.String())
				}
				return true
			})
		}
	}

}
