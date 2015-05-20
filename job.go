package main

import (
	"fmt"
)

type Route struct {
	Url    string
	Method string
}

type Job struct {
	Route
	Data interface{}
}

func (r Route) String() string {
	return fmt.Sprintf("Method: %s\tUrl: %s", r.Method, r.Url)
}
