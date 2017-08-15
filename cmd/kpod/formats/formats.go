package formats

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/pkg/errors"
)

// Writer interface for outputs
type Writer interface {
	Out() error
}

// JSONstruct for JSON output
type JSONstruct struct {
	Output []interface{}
}

// StdoutTemplate for Go template output
type StdoutTemplate struct {
	Output   []interface{}
	Template string
	Fields   map[string]string
}

// Out method for JSON
func (j JSONstruct) Out() error {
	data, err := json.MarshalIndent(j.Output, "", "    ")
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", data)
	return nil
}

// Out method for Go templates
func (t StdoutTemplate) Out() error {
	if strings.HasPrefix(t.Template, "table") {
		t.Template = strings.TrimSpace(t.Template[5:])
		headerTmpl, err := template.New("header").Funcs(headerFunctions).Parse(t.Template)
		if err != nil {
			errors.Wrapf(err, "Template parsing error")
		}
		err = headerTmpl.Execute(os.Stdout, t.Fields)
		fmt.Println()
	}
	tmpl, err := template.New("image").Funcs(basicFunctions).Parse(t.Template)
	if err != nil {
		return errors.Wrapf(err, "Template parsing error")
	}

	for _, img := range t.Output {
		basicTmpl := tmpl.Funcs(basicFunctions)
		err = basicTmpl.Execute(os.Stdout, img)
		if err != nil {
			return err
		}
		fmt.Println()
	}
	return nil

}
