package storage

import (
	"fmt"
)

type errors []error

func (err errors) Error() string { return fmt.Sprintf("%+v", []error(err)) } // TODO improve formatting?

func collectErrors(accum *error, new error) {
	if new == nil {
		return
	} else if *accum == nil {
		*accum = new
	} else if errs, ok := (*accum).(errors); ok {
		*accum = append(errs, new)
	} else {
		*accum = errors{*accum, new}
	}
}
