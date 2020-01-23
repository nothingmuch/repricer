package errors

import (
	"fmt"
)

type Temporary string

func (s Temporary) Error() string { return string(s) }
func (Temporary) Temporary() bool { return true }

type Errors []error

func (err Errors) Error() string { return fmt.Sprintf("%+v", []error(err)) } // TODO improve formatting?

func Collect(accum *error, new error) {
	if new == nil {
		return
	} else if *accum == nil {
		*accum = new
	} else if errs, ok := (*accum).(Errors); ok {
		*accum = append(errs, new)
	} else {
		*accum = Errors{*accum, new}
	}
}
