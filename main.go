package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/types"
	"golang.org/x/tools/go/loader" //nolint:staticcheck
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FuncDescriptor struct {
	PackageName  string
	Receiver     string
	FunctionName string
	FuncType     string
}

func (fd FuncDescriptor) Id() string {
	recvStr := fd.Receiver
	return fd.PackageName + recvStr + "." + fd.FunctionName + "." + fd.FuncType
}

func Contains(a []FuncDescriptor, x FuncDescriptor) bool {
	for _, n := range a {
		if x.Id() == n.Id() {
			return true
		}
	}
	return false
}

func getInterfaces(defs map[*ast.Ident]types.Object) map[string]types.Object {
	interfaces := make(map[string]types.Object)
	for id, obj := range defs {
		if obj == nil || obj.Type() == nil {
			continue
		}
		if _, ok := obj.(*types.TypeName); !ok {
			continue
		}
		if types.IsInterface(obj.Type()) {
			interfaces[id.Name] = obj
		}
	}
	return interfaces
}

func isAny(obj types.Object) bool {
	return obj.Type().String() == "any" || obj.Type().Underlying().String() == "any"
}

func getInterfaceNameForReceiver(interfaces map[string]types.Object, recv *types.Var) string {
	var recvInterface string
	for _, obj := range interfaces {
		if t, ok := obj.Type().Underlying().(*types.Interface); ok {
			if types.Implements(recv.Type(), t) && !isAny(obj) {
				recvInterface = "." + obj.Type().String()
			}
		}
	}
	return recvInterface
}

func findRootFunctions(file *ast.File, ginfo *types.Info, interfaces map[string]types.Object, functionLabel string, rootFunctions *[]FuncDescriptor) {
	var currentFun FuncDescriptor
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			ftype := ginfo.Defs[node.Name].Type()
			signature := ftype.(*types.Signature)
			recv := signature.Recv()

			var recvStr string
			var recvInterface string
			if recv != nil {
				recvStr = "." + recv.Type().String()
				recvInterface = getInterfaceNameForReceiver(interfaces, recv)
			}
			if recvInterface != "" {
				currentFun = FuncDescriptor{file.Name.Name, recvInterface, node.Name.String(), ftype.String()}
			} else {
				currentFun = FuncDescriptor{file.Name.Name, recvStr, node.Name.String(), ftype.String()}
			}

		case *ast.CallExpr:
			selector, ok := node.Fun.(*ast.SelectorExpr)
			if ok {
				if selector.Sel.Name == functionLabel {
					fmt.Println("sel:", selector.Sel.Name, currentFun)
					*rootFunctions = append(*rootFunctions, currentFun)
				}
			}
		}
		return true
	})
}

func findFuncDecls(file *ast.File, ginfo *types.Info, interfaces map[string]types.Object, funcDecls map[FuncDescriptor]bool) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			ftype := ginfo.Defs[node.Name].Type()
			signature := ftype.(*types.Signature)
			recv := signature.Recv()

			var recvStr string
			var recvInterface string
			if recv != nil {
				recvStr = "." + recv.Type().String()
				recvInterface = getInterfaceNameForReceiver(interfaces, recv)
			}
			if recvInterface != "" {
				funcDecl := FuncDescriptor{file.Name.Name, recvInterface, node.Name.String(), ftype.String()}
				funcDecls[funcDecl] = true
			}
			funcDecl := FuncDescriptor{file.Name.Name, recvStr, node.Name.String(), ftype.String()}
			funcDecls[funcDecl] = true
		}
		return true
	})
}

func dumpFuncDecls(funcDecls map[FuncDescriptor]bool) {
	fmt.Println("FuncDecls")
	for fun, _ := range funcDecls {
		fmt.Println(fun)
	}
}

func addFuncCallToCallGraph(funcCall FuncDescriptor, currentFun FuncDescriptor,
	funcDecls map[FuncDescriptor]bool, backwardCallGraph map[FuncDescriptor][]FuncDescriptor) {
	if !Contains(backwardCallGraph[funcCall], currentFun) {
		if _, ok := funcDecls[funcCall]; ok {
			backwardCallGraph[funcCall] = append(backwardCallGraph[funcCall], currentFun)
		}
	}
}

func buildCallGraph(file *ast.File, ginfo *types.Info,
	interfaces map[string]types.Object, funcDecls map[FuncDescriptor]bool,
	backwardCallGraph map[FuncDescriptor][]FuncDescriptor) {
	currentFun := FuncDescriptor{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			ftype := ginfo.Defs[node.Name].Type()
			signature := ftype.(*types.Signature)
			recv := signature.Recv()

			var recvStr string
			if recv != nil {
				recvStr = "." + recv.Type().String()
			}
			currentFun = FuncDescriptor{file.Name.Name, recvStr, node.Name.String(), ftype.String()}
		case *ast.CallExpr:
			switch node := node.Fun.(type) {
			case *ast.Ident:
				ftype := ginfo.Uses[node].Type()
				if ftype != nil {
					funcCall := FuncDescriptor{file.Name.Name, "", node.Name, ftype.String()}
					addFuncCallToCallGraph(funcCall, currentFun, funcDecls, backwardCallGraph)
				}
			case *ast.SelectorExpr:
				obj := ginfo.Selections[node]
				if obj != nil {
					recv := obj.Recv()
					var ftypeStr string
					// sel.Sel is function ident
					ftype := ginfo.Uses[node.Sel]

					if ftype != nil {
						ftypeStr = ftype.Type().String()
					}
					var recvStr string
					if len(recv.String()) > 0 {
						recvStr = "." + recv.String()
					}

					funcCall := FuncDescriptor{file.Name.Name, recvStr, obj.Obj().Name(), ftypeStr}
					addFuncCallToCallGraph(funcCall, currentFun, funcDecls, backwardCallGraph)
				}

			}
		}
		return true

	})
}

func dumpFuncCalls(file *ast.File, ginfo *types.Info) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			switch node := node.Fun.(type) {
			case *ast.Ident:
				ftype := ginfo.Uses[node].Type()
				if ftype != nil {
					funcCall := FuncDescriptor{file.Name.Name, "", node.Name, ftype.String()}
					fmt.Println("FuncCall:", funcCall)
				}
			case *ast.SelectorExpr:
				obj := ginfo.Selections[node]
				if obj != nil {
					recv := obj.Recv()
					var ftypeStr string
					// sel.Sel is function ident
					ftype := ginfo.Uses[node.Sel]

					if ftype != nil {
						ftypeStr = ftype.Type().String()
					}
					var recvStr string
					if len(recv.String()) > 0 {
						recvStr = "." + recv.String()
					}

					funcCall := FuncDescriptor{file.Name.Name, recvStr, obj.Obj().Name(), ftypeStr}
					fmt.Println("FuncCall:", funcCall)
				}

			}
		}
		return true

	})
}

func dumpCallGraph(backwardCallGraph map[FuncDescriptor][]FuncDescriptor) {
	fmt.Println("\n\tchild parent")
	for k, v := range backwardCallGraph {
		fmt.Print("\n\t", k.Id())
		fmt.Print(" ", v)
	}
	fmt.Print("\n")
}

func FindRootFunctions(prog *loader.Program, ginfo *types.Info, interfaces map[string]types.Object, allowedPathPattern string) []FuncDescriptor {
	var rootFunctions []FuncDescriptor
	for _, pkg := range prog.AllPackages {

		fmt.Printf("Package path %q\n", pkg.Pkg.Path())
		for _, file := range pkg.Files {
			if allowedPathPattern != "" && !strings.Contains(prog.Fset.Position(file.Name.Pos()).String(), allowedPathPattern) {
				continue
			}
			fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
			findRootFunctions(file, ginfo, interfaces, "AutotelEntryPoint", &rootFunctions)
		}
	}
	return rootFunctions
}

func FindFuncDecls(prog *loader.Program, ginfo *types.Info, interfaces map[string]types.Object, allowedPathPattern string) map[FuncDescriptor]bool {
	funcDecls := make(map[FuncDescriptor]bool)
	for _, pkg := range prog.AllPackages {

		fmt.Printf("Package path %q\n", pkg.Pkg.Path())
		for _, file := range pkg.Files {
			if allowedPathPattern != "" && !strings.Contains(prog.Fset.Position(file.Name.Pos()).String(), allowedPathPattern) {
				continue
			}
			fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
			findFuncDecls(file, ginfo, interfaces, funcDecls)
		}
	}
	return funcDecls
}

func BuildCallGraph(prog *loader.Program, ginfo *types.Info,
	interfaces map[string]types.Object, funcDecls map[FuncDescriptor]bool, allowedPathPattern string) map[FuncDescriptor][]FuncDescriptor {
	backwardCallGraph := make(map[FuncDescriptor][]FuncDescriptor)
	for _, pkg := range prog.AllPackages {
		fmt.Printf("Package path %q\n", pkg.Pkg.Path())
		for _, file := range pkg.Files {
			if allowedPathPattern != "" && !strings.Contains(prog.Fset.Position(file.Name.Pos()).String(), allowedPathPattern) {
				continue
			}
			fmt.Println(prog.Fset.Position(file.Name.Pos()).String())
			buildCallGraph(file, ginfo, interfaces, funcDecls, backwardCallGraph)
		}
	}
	return backwardCallGraph
}

func usage() {
	fmt.Println("usage loader [main package path] [allowed path pattern]")
}

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	var allowedPathPattern string

	if len(os.Args) == 3 {
		allowedPathPattern = os.Args[2]
	}

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
	ginfo := &types.Info{
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	var mutex = &sync.RWMutex{}
	conf.AfterTypeCheck = func(info *loader.PackageInfo, files []*ast.File) {
		for k, v := range info.Defs {
			mutex.Lock()
			ginfo.Defs[k] = v
			mutex.Unlock()
		}
		for k, v := range info.Uses {
			mutex.Lock()
			ginfo.Uses[k] = v
			mutex.Unlock()
		}
		for k, v := range info.Selections {
			mutex.Lock()
			ginfo.Selections[k] = v
			mutex.Unlock()
		}
	}
	prog, err := conf.Load()
	if err != nil {
		fmt.Println(err)
	}
	interfaces := getInterfaces(ginfo.Defs)

	rootFunctions := FindRootFunctions(prog, ginfo, interfaces, allowedPathPattern)
	funcDecls := FindFuncDecls(prog, ginfo, interfaces, allowedPathPattern)
	backwardCallGraph := BuildCallGraph(prog, ginfo, interfaces, funcDecls, allowedPathPattern)

	fmt.Println("RootFunctions: ", rootFunctions)
	dumpFuncDecls(funcDecls)
	dumpCallGraph(backwardCallGraph)
	fmt.Println("Callgraph:", len(backwardCallGraph))
}
