package pacaroute

import (
	"fmt"
	"strings"
)

type arguments struct {
	words []string
	flags map[string]string
}

func parseArgs(args []string) (arguments, error) {
	v := arguments{flags: map[string]string{}}
	bools := map[string]bool{"apply": true, "local": true, "allow-local": true, "local-management": true, "json": true, "no-open": true, "no-serve": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "--") {
			v.words = append(v.words, arg)
			continue
		}
		name, value, has := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
		if name == "help" {
			v.words = []string{"help"}
			return v, nil
		}
		if bools[name] {
			if !has {
				value = "true"
			}
		} else if !has {
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") {
				return v, fmt.Errorf("missing --%s value", name)
			}
			i++
			value = args[i]
		}
		v.flags[name] = value
	}
	return v, nil
}
func (a arguments) get(k, d string) string {
	if v, ok := a.flags[k]; ok {
		return v
	}
	return d
}
