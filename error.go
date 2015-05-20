package main

import (
	"fmt"
)

type AttrError struct {
	Url       string
	Selection string
	Attribute string
}

func (e AttrError) Error() string {
	return fmt.Sprintf(
		"NotFound %s %s at %s", e.Selection, e.Attribute, e.Url)
}
