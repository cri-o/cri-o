package formats

import (
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"os"
	"text/template"
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

	tmpl, err := template.New("image").Parse(t.Template)
	if err != nil {
		return errors.Wrapf(err, "Template parsing error")
	}

	for _, img := range t.Output {
		err = tmpl.Execute(os.Stdout, img)
		if err != nil {
			return err
		}
		fmt.Println()
	}
	return nil

}
