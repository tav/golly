// Public Domain (-) 2013 The Golly Authors.
// See the Golly UNLICENSE file for details.

package main

import (
	"bytes"
	"fmt"
	"github.com/tav/golly/fsutil"
	"github.com/tav/golly/log"
	"github.com/tav/golly/optparse"
	"github.com/tav/golly/runtime"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

type fieldInfo struct {
	dbName string
	kind   string
	name   string
}

type model struct {
	fields []fieldInfo
	name   string
}

var header = []byte(`// DO NOT EDIT.
// Auto-generated template file by dynamodb-marshal.

package `)

var kindMap = map[string]string{
	"[]byte":   "B",
	"int":      "N",
	"int64":    "N",
	"string":   "S",
	"time":     "N",
	"uint":     "N",
	"uint64":   "N",
	"[][]byte": "BS",
	"[]int":    "NS",
	"[]int64":  "NS",
	"[]string": "SS",
}

func parseFile(path string, force bool) {

	dir, filename := filepath.Split(path)
	if !strings.HasSuffix(filename, ".go") {
		runtime.Error("%s does not look like a go file", filename)
	}

	log.Info("Parsing %s", path)
	fset := token.NewFileSet()
	pkg, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		runtime.StandardError(err)
	}

	newpath := filepath.Join(dir, fmt.Sprintf("%s_marshal.go", filename[:len(filename)-3]))
	exists, err := fsutil.FileExists(newpath)
	if exists && !force {
		runtime.Error("%s already exists! please specify --force to overwrite", newpath)
	}

	prev := ""
	models := []*model{}

	ast.Inspect(pkg, func(n ast.Node) bool {
		if s, ok := n.(*ast.StructType); ok {
			fields := []fieldInfo{}
			for _, field := range s.Fields.List {
				if field.Names == nil {
					continue
				}
				name := field.Names[0].Name
				dbName := ""
				kind := ""
				if field.Tag != nil {
					tag := field.Tag.Value[1 : len(field.Tag.Value)-1]
					if tag == "-" {
						continue
					}
					dbName = tag
				}
				if dbName == "" {
					dbName = name
					rune, _ := utf8.DecodeRuneInString(name)
					if !unicode.IsUpper(rune) {
						continue
					}
				}
				switch expr := field.Type.(type) {
				case *ast.Ident:
					switch expr.Name {
					case "string", "int", "int64", "uint", "uint64":
						kind = expr.Name
					}
				case *ast.ArrayType:
					if expr.Len == nil { // slice type
						switch iexpr := expr.Elt.(type) {
						case *ast.Ident:
							switch iexpr.Name {
							case "byte", "string", "int", "int64", "uint", "uint64":
								kind = "[]" + iexpr.Name
							}
						case *ast.ArrayType:
							if iexpr.Len == nil {
								if iiexpr, ok := iexpr.Elt.(*ast.Ident); ok {
									if iiexpr.Name == "byte" {
										kind = "[][]byte"
									}
								}
							}
						}
					}
				case *ast.SelectorExpr:
					if lexpr, ok := expr.X.(*ast.Ident); ok {
						if lexpr.Name == "time" && expr.Sel.Name == "Time" {
							kind = "time"
						}
					}
				}
				if kind == "" {
					log.Error("unsupported: %v field (%s.%s)", field.Type, prev, name)
					continue
				}
				fields = append(fields, fieldInfo{
					dbName: dbName,
					kind:   kind,
					name:   name,
				})
			}
			model := &model{
				fields: fields,
				name:   prev,
			}
			models = append(models, model)
		}
		switch x := n.(type) {
		case *ast.Ident:
			prev = x.Name
		}
		return true
	})

	buf := &bytes.Buffer{}
	buf.Write(header)
	buf.Write([]byte(pkg.Name.Name))
	buf.Write([]byte("\n\nimport (\n\t\"bytes\"\n\t\"encoding/base64\"\n\t\"io\"\n\t\"strconv\"\n\t\"time\"\n\t\"unicode/utf8\"\n)\n\n"))

	for _, model := range models {
		ref := strings.ToLower(string(model.name[0]))
		fmt.Fprintf(buf, "func (%s *%s) Encode(buf *bytes.Buffer) {\n", ref, model.name)
		last := len(model.fields) - 1
		close := `{"`
		written := false
		for idx, field := range model.fields {
			dbKind, ok := kindMap[field.kind]
			if !ok {
				log.Error("unsupported kind: %s", field.kind)
				continue
			}
			prefix := `"`
			suffix := `"`
			if len(dbKind) == 2 {
				prefix = "["
				suffix = "]"
			}
			open := fmt.Sprintf(`%s%s":{"%s":%s`, close, field.dbName, dbKind, prefix)
			comma := ","
			if idx == last {
				comma = ""
			}
			fmt.Fprintf(buf, "\tbuf.WriteString(`%s`)\n", open)
			close = fmt.Sprintf(`%s}%s"`, suffix, comma)
			written = true
			selector := fmt.Sprintf("%s.%s", ref, field.name)
			if len(dbKind) == 2 {
				fmt.Fprintf(buf, "\tfor idx, elem := range %s {\n", selector)
				fmt.Fprint(buf, "\t\tbuf.WriteByte('\"')\n")
				write(buf, "\t\t", field.kind[2:], "elem")
				fmt.Fprintf(buf, "\t\tif idx == len(%s)-1 {\n", selector)
				fmt.Fprint(buf, "\t\t\tbuf.WriteByte('\"')\n")
				fmt.Fprint(buf, "\t\t} else {\n")
				fmt.Fprint(buf, "\t\t\tbuf.WriteString(`\",`)\n")
				fmt.Fprint(buf, "\t\t}\n")
				fmt.Fprint(buf, "\t}\n")
			} else {
				write(buf, "\t", field.kind, selector)
			}
		}
		if written {
			fmt.Fprintf(buf, "\tbuf.WriteString(`%s}`)\n", close[:len(close)-1])
		}
		fmt.Fprintf(buf, "}\n\n")
		fmt.Fprintf(buf, "func (%s *%s) Decode(data map[string]map[string]interface{}) {\n", ref, model.name)
		close = ""
		for idx, field := range model.fields {
			dbKind, ok := kindMap[field.kind]
			if !ok {
				continue
			}
			selector := fmt.Sprintf("%s.%s", ref, field.name)
			if len(dbKind) == 2 {
				fmt.Fprintf(buf, "%s\tif vals, ok := data[\"%s\"][\"%s\"].([]string); ok {\n", close, field.dbName, dbKind)
				fmt.Fprint(buf, "\t\tfor _, val := range vals {\n")
				readMulti(buf, "\t\t\t", field.kind, selector)
				fmt.Fprint(buf, "\t\t}\n")
			} else {
				fmt.Fprintf(buf, "%s\tif val, ok := data[\"%s\"][\"%s\"].(string); ok {\n", close, field.dbName, dbKind)
				read(buf, "\t\t", field.kind, selector)
			}
			_ = idx
			close = "\t}\n"
		}
		fmt.Fprintf(buf, "%s}\n\n", close)
	}

	buf.Write(jsonSupport)

	log.Info("Writing %s", newpath)
	newfile, err := os.Create(newpath)
	if err != nil {
		runtime.StandardError(err)
	}

	newfile.Write(buf.Bytes())
	newfile.Close()

}

func read(buf *bytes.Buffer, lead, kind, selector string) {
	switch kind {
	case "[]byte":
		fmt.Fprintf(buf, "%s%s, _ := base64.StdEncoding.DecodeString(val)\n", lead, selector)
	case "string":
		fmt.Fprintf(buf, "%s%s = val\n", lead, selector)
	case "int":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseInt(val, 10, 64)\n", lead)
		fmt.Fprintf(buf, "%s%s = int(tmp)\n", lead, selector)
	case "int64":
		fmt.Fprintf(buf, "%s%s, _ = strconv.ParseInt(val, 10, 64)\n", lead, selector)
	case "uint":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseUint(val, 10, 64)\n", lead)
		fmt.Fprintf(buf, "%s%s = uint(tmp)\n", lead, selector)
	case "uint64":
		fmt.Fprintf(buf, "%s%s, _ = strconv.ParseUint(val, 10, 64)\n", lead, selector)
	case "time":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseInt(val, 10, 64)\n", lead)
		fmt.Fprintf(buf, "%s%s = time.Unix(0, tmp).UTC()\n", lead, selector)
	}
}

func readMulti(buf *bytes.Buffer, lead, kind, selector string) {
	switch kind {
	case "[][]byte":
		fmt.Fprintf(buf, "%stmp, _ := base64.StdEncoding.DecodeString(val)\n", lead)
		fmt.Fprintf(buf, "%s%s = append(%s, tmp)\n", lead, selector, selector)
	case "[]string":
		fmt.Fprintf(buf, "%s%s = append(%s, val)\n", lead, selector, selector)
	case "[]int":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseInt(val, 10, 64)\n", lead)
		fmt.Fprintf(buf, "%s%s = append(%s, int(tmp))\n", lead, selector, selector)
	case "[]int64":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseInt(val, 10, 64)\n", lead, selector)
		fmt.Fprintf(buf, "%s%s = append(%s, tmp)\n", lead, selector, selector)
	case "[]uint":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseUint(val, 10, 64)\n", lead, selector)
		fmt.Fprintf(buf, "%s%s = append(%s, uint(tmp))\n", lead, selector, selector)
	case "[]uint64":
		fmt.Fprintf(buf, "%stmp, _ := strconv.ParseUint(val, 10, 64)\n", lead, selector)
		fmt.Fprintf(buf, "%s%s = append(%s, tmp)\n", lead, selector, selector)
	}
}

func write(buf *bytes.Buffer, lead, kind, selector string) {
	switch kind {
	case "[]byte":
		fmt.Fprintf(buf, "%sbuf.WriteString(base64.StdEncoding.EncodeToString(%s))\n", lead, selector)
	case "string":
		fmt.Fprintf(buf, "%stoJSON(%s, buf)\n", lead, selector)
	case "int":
		fmt.Fprintf(buf, "%sbuf.WriteString(strconv.FormatInt(int64(%s), 10))\n", lead, selector)
	case "int64":
		fmt.Fprintf(buf, "%sbuf.WriteString(strconv.FormatInt(%s, 10))\n", lead, selector)
	case "uint":
		fmt.Fprintf(buf, "%sbuf.WriteString(strconv.FormatUint(int64(%s), 10))\n", lead, selector)
	case "uint64":
		fmt.Fprintf(buf, "%sbuf.WriteString(strconv.FormatUint(%s, 10))\n", lead, selector)
	case "time":
		fmt.Fprintf(buf, "%sbuf.WriteString(strconv.FormatInt(%s.UnixNano(), 10))\n", lead, selector)
	}
}

var jsonSupport = []byte(`
// Adapted from the encoding/json package in the standard
// library.
const hex = "0123456789abcdef"

func toJSON(s string, buf *bytes.Buffer) {
	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if 0x20 <= b && b != '\\' && b != '"' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			if start < i {
				buf.WriteString(s[start:i])
			}
			switch b {
			case '\\', '"':
				buf.WriteByte('\\')
				buf.WriteByte(b)
			case '\n':
				buf.WriteByte('\\')
				buf.WriteByte('n')
			case '\r':
				buf.WriteByte('\\')
				buf.WriteByte('r')
			default:
				buf.WriteString("\\u00")
				buf.WriteByte(hex[b>>4])
				buf.WriteByte(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				buf.WriteString(s[start:i])
			}
			buf.WriteString("\\ufffd")
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		buf.WriteString(s[start:])
	}
}
`)

func main() {

	opts := optparse.New("Usage: dynamodb-marshal file1.go [file2.go ...]",
		"dynamodb-marshal 0.0.1")

	force := opts.Bool([]string{"-f", "--force"},
		"overwrite existing marshal files")

	os.Args[0] = "dynamodb-marshal"
	files := opts.Parse(os.Args)

	if len(files) == 0 {
		opts.PrintUsage()
		runtime.Exit(0)
	}

	log.AddConsoleLogger()
	for _, file := range files {
		path, err := filepath.Abs(file)
		if err != nil {
			runtime.StandardError(err)
		}
		parseFile(path, *force)
	}

	log.Wait()

}
