package gonerator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/corsc/go-tools/gonerator/tmpl"
)

// Gonerator co-ordinates the generation of code
type Gonerator struct {
	buf  bytes.Buffer
	pkg  *Package
	data tmpl.TemplateData
}

// Build generates the code based on supplied values (request call to preceeding ParsePackageDir()
func (g *Gonerator) Build(dir string, typeName string, templateFile string, outputFile string, extras string) {
	g.data = tmpl.TemplateData{
		TypeName:     typeName,
		TemplateFile: templateFile,
		OutputFile:   outputFile,

		Fields:  []tmpl.Field{},
		Extras:  []string{},
		Methods: []tmpl.Method{},
	}

	g.buildHeader()

	var path string
	if !strings.HasPrefix(templateFile, "/") {
		path = dir + templateFile
	} else {
		path = templateFile
	}

	templateContent, err := ioutil.ReadFile(path)
	if err != nil {
		panic(err)
	}

	g.buildTemplateData(extras)

	g.generate(string(templateContent))
	err = g.writeFile(dir, outputFile, typeName)
	if err != nil {
		os.Exit(-1)
	}
}

func (g *Gonerator) buildTemplateData(extras string) {
	g.findTypeFields()

	g.data.Extras = strings.Split(extras, ",")
	g.data.PackageName = g.pkg.name
}

func (g *Gonerator) findTypeFields() {
	for _, file := range g.pkg.astFiles {
		fields := tmpl.GetFields(file, g.data.TypeName)
		if len(fields) > 0 {
			g.data.Fields = append(g.data.Fields, fields...)
		}
		methods := tmpl.GetMethods(file, g.data.TypeName)
		if len(methods) > 0 {
			g.data.Methods = append(g.data.Methods, methods...)
		}
	}
}

// generate produces the code for the named type.
func (g *Gonerator) generate(templateContent string) {
	buffer := &bytes.Buffer{}
	tmpl.Generate(buffer, g.data, templateContent)
	g.printf(buffer.String())
}

func (g *Gonerator) printf(format string, args ...interface{}) {
	fmt.Fprintf(&g.buf, format, args...)
}

// ParsePackageDir parses the package residing in the directory.
func (g *Gonerator) ParsePackageDir(directory string) {
	pkg, err := build.Default.ImportDir(directory, 0)
	if err != nil {
		log.Fatalf("[%s] cannot process directory %s: %s", g.data.OutputFile, directory, err)
	}
	var names []string
	names = append(names, pkg.GoFiles...)
	names = prefixDirectory(directory, names)
	g.parsePackage(directory, names)
}

// prefixDirectory places the directory name on the beginning of each name in the list.
func prefixDirectory(directory string, names []string) []string {
	if directory == "." {
		return names
	}
	ret := make([]string, len(names))
	for i, name := range names {
		ret[i] = filepath.Join(directory, name)
	}
	return ret
}

// parsePackage analyzes the single package constructed from the named files.
// If text is non-nil, it is a string to be used instead of the content of the file,
// to be used for testing. parsePackage exits if there is an error.
func (g *Gonerator) parsePackage(directory string, names []string) {
	var files []*File
	var astFiles []*ast.File
	g.pkg = &Package{}
	fs := token.NewFileSet()
	for _, name := range names {
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		parsedFile, err := parser.ParseFile(fs, name, nil, 0)
		if err != nil {
			log.Fatalf("[%s] parsing package: %s: %s", g.data.OutputFile, name, err)
		}
		astFiles = append(astFiles, parsedFile)
		files = append(files, &File{
			file: parsedFile,
			pkg:  g.pkg,
		})
	}
	if len(astFiles) == 0 {
		log.Fatalf("[%s] %s: no buildable Go files", g.data.OutputFile, directory)
	}
	g.pkg.name = astFiles[0].Name.Name
	g.pkg.files = files
	g.pkg.dir = directory
	g.pkg.astFiles = astFiles
}

// format returns the gofmt-ed contents of the Gonerator's buffer.
func (g *Gonerator) format() ([]byte, error) {
	src, err := format.Source(g.buf.Bytes())
	if err != nil {
		// Should never happen, but can arise when developing this code.
		// The user can compile the output to see the error.
		log.Printf("[%s] warning: internal error: invalid Go gonerated: %s", g.data.OutputFile, err)
		log.Printf("[%s] warning: compile the package to analyze the error", g.data.OutputFile)
		return g.buf.Bytes(), err
	}
	return src, nil
}

func (g *Gonerator) toBytes() []byte {
	return g.buf.Bytes()
}

func (g *Gonerator) buildHeader() {
	if isGo(g.data.OutputFile) {
		g.printf("// Code gonerated by \"github.com/myteksi/go/tools/gonerator\"\n// DO NOT EDIT\n")
		g.printf("// @" + "generated \n") // this is separate to fool phabricator
		g.printf("//\n")
		g.printf("// Args:\n")
		g.printf("// TypeName: %s\n", g.data.TypeName)
		g.printf("// Template: %s\n", g.data.TemplateFile)
		g.printf("// Destination: %s\n", g.data.OutputFile)
		g.printf("\n")
	}
}

func (g *Gonerator) writeFile(dir string, filename string, typeName string) error {
	var src []byte
	var errOut error
	if isGo(filename) {
		src, errOut = g.format()
	} else {
		src = g.toBytes()
	}

	outputName := filepath.Join(dir, strings.ToLower(filename))

	directory := filepath.Dir(outputName)
	err := os.MkdirAll(directory, 0755)
	if err != nil {
		log.Fatalf("[%s] error creating destination directory: %s", g.data.OutputFile, err)
	}

	err = ioutil.WriteFile(outputName, src, 0644)
	if err != nil {
		log.Fatalf("[%s] error writing output: %s", g.data.OutputFile, err)
	}

	return errOut
}

func isGo(filename string) bool {
	return strings.Contains(filename, ".go")
}