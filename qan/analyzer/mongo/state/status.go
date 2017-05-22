package state

import (
	"fmt"

	"github.com/fatih/structs"
)

func StatusToMap(in interface{}) map[string]string {
	out := map[string]string{}
	for _, f := range structs.New(in).Fields() {
		if f.IsZero() {
			continue
		}
		tag := f.Tag("name")
		if tag == "" {
			continue
		}
		out[tag] = fmt.Sprint(f.Value())
	}
	return out
}
