/********************************
*** Multiplexer for Go        ***
*** Bone is under MIT license ***
*** Code by CodingFerret      ***
*** github.com/go-zoo         ***
*********************************/

package bone

// Validator can be passed to a route to validate the params
type Validator interface {
	Validate(string) bool
}

type validatorFunc struct {
	validateFunc func(string) bool
}

func newValidatorFunc(v func(string) bool) validatorFunc {
	return validatorFunc{validateFunc: v}
}

func (v validatorFunc) Validate(s string) bool {
	return v.validateFunc(s)
}

type validatorInfo struct {
	start int
	end   int
	name  string
}

func containsValidators(path string) []validatorInfo {
	var index []int
	for i, c := range path {
		if c == '|' {
			index = append(index, i)
		}
	}

	if len(index) > 0 {
		var validators []validatorInfo
		for i, pos := range index {
			if i+1 == len(index) {
				validators = append(validators, validatorInfo{
					start: pos,
					end:   len(path),
					name:  path[pos:len(path)],
				})
			} else {
				validators = append(validators, validatorInfo{
					start: pos,
					end:   index[i+1],
					name:  path[pos:index[i+1]],
				})
			}
		}
		return validators
	}
	return nil
}
