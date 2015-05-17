package main

import (
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"
)

type Author struct {
	Id   string
	Name string
}

func (a Author) String() string {
	return fmt.Sprintf("Author: %s-%s", a.Id, a.Name)
}

type Illust struct {
	Id   string
	Name string
	Author
}

func (i Illust) String() string {
	return fmt.Sprintf("Illust: %s-%s %s", i.Id, i.Name, i.Author.String())
}

type Image struct {
	Id      int
	Path    string
	Referer string
	Illust
}

func (i Image) String() string {
	return fmt.Sprintf("%s Image: %s", i.Illust, i.Path)
}

func (img Image) Format(format string, isDir bool) string {
	format = strings.Replace(format, "{{Illust.Id}}", img.Illust.Id, -1)
	format = strings.Replace(format, "{{Illust.Name}}", img.Illust.Name, -1)
	format = strings.Replace(format, "{{Author.Id}}", img.Illust.Author.Id, -1)
	format = strings.Replace(format, "{{Author.Name}}", img.Illust.Author.Name, -1)
	format = strings.Replace(format, "{{Image.Id}}", strconv.Itoa(img.Id), -1)

	if !isDir {
		u, _ := url.Parse(img.Path)
		format += path.Ext(u.Path)
	}

	format = strings.Replace(format, "/", "SLASH", -1)

	return format
}
