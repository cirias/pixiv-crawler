package main

import (
	//"fmt"
	"github.com/PuerkitoBio/goquery"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

func PanicIf(err error) {
	if err != nil {
		panic(err)
	}
}

type Crawler struct {
	*http.Client
	addJober reflect.Value
}

func (c *Crawler) addJob(j Job) {
	c.addJober.Call([]reflect.Value{reflect.ValueOf(j)})
}

func (c *Crawler) Login(j Job) error {
	cert := j.Data.(Certification)
	form := url.Values{
		"mode":     {"login"},
		"pixiv_id": {cert.username},
		"pass":     {cert.password},
		"skip":     {"1"},
	}
	req, _ := http.NewRequest(
		"POST",
		j.Route.Url,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html, application/xhtml+xml, */*")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Host", "www.secure.pixiv.net")
	req.Header.Set("Referer", "http://www.pixiv.net/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36")

	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Prepare author page url
	u, err := url.Parse(AUTHOR_URL)
	PanicIf(err)
	q := u.Query()
	q.Set("id", memberId)
	u.RawQuery = q.Encode()

	c.addJob(Job{Route: Route{u.String(), "GetAuthor"}})

	return nil
}

func (c *Crawler) resquest(u string) (doc *goquery.Document, err error) {
	res, err := c.Get(u)
	if err != nil {
		return nil, err
	}

	doc, err = goquery.NewDocumentFromResponse(res)
	return
}

func (c *Crawler) GetAuthor(j Job) error {
	doc, err := c.resquest(j.Route.Url)
	if err != nil {
		return err
	}

	// Get author's name
	name := doc.Find("h1.user").First().Text()

	// Get author's id
	u, err := url.Parse(j.Route.Url)
	PanicIf(err)
	id := u.Query().Get("id")

	// Get works count and page count
	text := doc.Find("span.count-badge").First().Text()
	re := regexp.MustCompile("[0-9]+")
	counts := re.FindAllString(text, -1)
	count, err := strconv.Atoi(counts[0])
	PanicIf(err)
	pages := count/ILLUST_SIZE + 1
	log.Println("Illust Count:", count, "Page Count:", pages)

	u, err = url.Parse(ILLUST_URL)
	PanicIf(err)
	q := u.Query()
	q.Set("id", id)

	// Build url of each page
	for i := 1; i <= pages; i++ {
		q.Set("p", strconv.Itoa(i))
		u.RawQuery = q.Encode()
		url := u.String()

		c.addJob(Job{Route{url, "GetIllusts"}, Author{id, name}})
	}
	return nil
}

func (c *Crawler) GetIllusts(j Job) error {
	doc, err := c.resquest(j.Route.Url)
	if err != nil {
		return err
	}

	selection := "ul._image-items>li.image-item"
	items := doc.Find(selection)

	items.Each(func(_ int, s *goquery.Selection) {
		subSelection := "a.work"
		attr := "href"
		href, exists := s.Find(subSelection).Attr(attr)

		if !exists {
			err := AttrError{
				j.Route.Url,
				selection + " " + subSelection,
				attr}
			PanicIf(err)
		} else {
			url := PIXIV_HOST + href

			c.addJob(Job{Route{url, "GetIllust"}, j.Data})
		}
	})

	return nil
}

func (c *Crawler) GetIllust(j Job) error {
	doc, err := c.resquest(j.Route.Url)
	if err != nil {
		return err
	}

	// Parse url, find illust id
	u, _ := url.Parse(j.Route.Url)
	id := u.Query().Get("illust_id")

	// Find illust name
	name := doc.Find("div.ui-expander-target>h1.title").First().Text()

	illust := Illust{id, name, j.Data.(Author)}

	if doc.Find("div.multiple").Length() != 0 {
		// Find next url
		selection := "div.works_display>a"
		attr := "href"
		path, exists := doc.Find(selection).First().Attr(attr)
		if !exists {
			err := AttrError{
				j.Route.Url,
				selection,
				attr}
			PanicIf(err)
		}

		url := PIXIV_HOST + "/" + path

		c.addJob(Job{Route{url, "GetMulti"}, illust})
	} else {
		selection := "img.original-image"
		attr := "data-src"
		src, exists := doc.Find(selection).Attr(attr)
		if !exists {
			if n := doc.Find("div.works_display div.player").Length(); n == 0 {
				err := AttrError{
					j.Route.Url,
					selection,
					attr}
				PanicIf(err)
			}
		} else {
			img := Image{Id: 0, Path: src, Illust: illust, Referer: j.Route.Url}

			c.addJob(Job{Route{Url: src, Method: "Download"}, img})
		}
	}

	return nil
}

func (c *Crawler) GetMulti(j Job) error {
	doc, err := c.resquest(j.Route.Url)
	if err != nil {
		return err
	}

	selection := "div.item-container"
	doc.Find(selection).Each(func(i int, s *goquery.Selection) {
		subSelection := "a"
		attr := "href"
		href, exists := s.Find(subSelection).Attr(attr)
		if !exists {
			err := AttrError{
				j.Route.Url,
				selection,
				attr}
			PanicIf(err)
		} else {
			image := Image{Id: i, Illust: j.Data.(Illust)}

			url := PIXIV_HOST + href

			c.addJob(Job{Route{url, "GetMultiFurther"}, image})
		}
	})

	return nil
}

func (c *Crawler) GetMultiFurther(j Job) error {
	doc, err := c.resquest(j.Route.Url)
	if err != nil {
		return err
	}

	selection := "img"
	doc.Find(selection).Each(func(_ int, s *goquery.Selection) {
		attr := "src"
		src, exists := s.Attr(attr)
		if !exists {
			err := AttrError{
				j.Route.Url,
				selection,
				attr}
			PanicIf(err)
		} else {
			image := j.Data.(Image)
			image.Path = src
			image.Referer = j.Route.Url

			c.addJob(Job{Route{Url: src, Method: "Download"}, image})
		}
	})

	return nil
}

func (c *Crawler) Download(j Job) error {
	image := j.Data.(Image)

	dirname := path.Join(dir, image.Format(dirformat, true))
	filename := path.Join(dirname, image.Format(fileformat, false))

	os.MkdirAll(dirname, 0777)
	file, err := os.Create(filename)
	PanicIf(err)
	defer file.Close()

	req, _ := http.NewRequest("GET", j.Route.Url, nil)
	req.Header.Set("Accept", "image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Host", "i2.pixiv.net")
	req.Header.Set("Referer", image.Referer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36")

	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if _, err = io.Copy(file, res.Body); err != nil {
		return err
	}

	return nil
}
