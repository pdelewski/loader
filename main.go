package main

import (
	"fmt"
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
	_, err = conf.Load()
	if err != nil {
	  fmt.Println(err)
	}

}
