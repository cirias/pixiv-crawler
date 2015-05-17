package main

import (
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/cenkalti/backoff"
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

type Route struct {
	Url    string
	Method string
}

func (r Route) String() string {
	return fmt.Sprintf("Method: %s\tUrl: %s", r.Method, r.Url)
}

type Job struct {
	Route
	Data interface{}
}

type Client struct {
	*http.Client
	addJob reflect.Value
}

func (c *Client) AddJob(j Job) {
	c.addJob.Call([]reflect.Value{reflect.ValueOf(j)})
}

func (c *Client) Login(id string, pass string) {
	form := url.Values{
		"mode":     {"login"},
		"pixiv_id": {id},
		"pass":     {pass},
		"skip":     {"1"},
	}
	req, _ := http.NewRequest(
		"POST",
		"https://www.secure.pixiv.net/login.php",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "text/html, application/xhtml+xml, */*")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Host", "www.secure.pixiv.net")
	req.Header.Set("Referer", "http://www.pixiv.net/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36")

	b := backoff.NewExponentialBackOff()
	t := backoff.NewTicker(b)

	var res *http.Response
	var err error
	for range t.C {
		if res, err = c.Do(req); err != nil {
			log.Println("Error:", err, "will retry...")
			continue
		}
		defer res.Body.Close()

		t.Stop()
		break
	}

	res.Body.Close()
}

func (c *Client) GetUrl(u string) (doc *goquery.Document, err error) {
	b := backoff.NewExponentialBackOff()
	t := backoff.NewTicker(b)

	var res *http.Response
	for range t.C {
		if res, err = c.Client.Get(u); err != nil {
			log.Println("Error:", err, "Url:", u, "will retry...")
			continue
		}
		defer res.Body.Close()

		t.Stop()
		break
	}

	doc, _ = goquery.NewDocumentFromResponse(res)
	return
}

func (c *Client) GetAuthor(j Job) {
	doc, _ := c.GetUrl(j.Route.Url)

	// Get author's name
	name := doc.Find("h1.user").First().Text()

	// Get author's id
	u, _ := url.Parse(j.Route.Url)
	id := u.Query().Get("id")

	// Get works count and page count
	text := doc.Find("span.count-badge").First().Text()
	re := regexp.MustCompile("[0-9]+")
	counts := re.FindAllString(text, -1)
	count, _ := strconv.Atoi(counts[0])
	pages := count/IllustsSize + 1
	log.Println("Illust Count:", count, "Page Count:", pages)

	u, _ = url.Parse(IllustUrl)
	q := u.Query()
	q.Set("id", id)

	// Build url of each page
	for i := 1; i <= pages; i++ {
		q.Set("p", strconv.Itoa(i))
		u.RawQuery = q.Encode()
		url := u.String()

		c.AddJob(Job{Route{url, "GetIllusts"}, Author{id, name}})
	}
}

func (c *Client) GetIllusts(j Job) {
	doc, _ := c.GetUrl(j.Route.Url)

	items := doc.Find("ul._image-items>li.image-item")

	items.Each(func(_ int, s *goquery.Selection) {
		href, exists := s.Find("a.work").Attr("href")

		if !exists {
			log.Println("Attribute 'href' dose not exist")
		} else {
			url := PixivHost + href

			c.AddJob(Job{Route{url, "GetIllust"}, j.Data})
		}
	})
}

func (c *Client) GetIllust(j Job) {
	doc, _ := c.GetUrl(j.Route.Url)

	// Parse url, find illust id
	u, _ := url.Parse(j.Route.Url)
	id := u.Query().Get("illust_id")

	// Find illust name
	name := doc.Find("div.ui-expander-target>h1.title").First().Text()

	illust := Illust{id, name, j.Data.(Author)}

	if doc.Find("div.multiple").Length() != 0 {
		// Find next url
		path, exists := doc.Find("div.works_display>a").First().Attr("href")
		if !exists {
			log.Fatalln("href not found")
		}

		url := PixivHost + "/" + path

		c.AddJob(Job{Route{url, "GetMulti"}, illust})
	} else {
		src, exists := doc.Find("img.original-image").Attr("data-src")
		if !exists {
			log.Fatalln(j, "Attribute 'data-src' dose not exist")
		} else {
			image := Image{Id: 0, Path: src, Illust: illust, Referer: j.Route.Url}

			c.AddJob(Job{Route{Url: src, Method: "Download"}, image})
		}
	}
}

func (c *Client) GetMulti(j Job) {
	doc, _ := c.GetUrl(j.Route.Url)

	doc.Find("div.item-container").Each(func(i int, s *goquery.Selection) {
		href, exists := s.Find("a").Attr("href")
		if !exists {
			log.Println("Attribute 'href' dose not exists")
		} else {
			image := Image{Id: i, Illust: j.Data.(Illust)}

			url := PixivHost + href

			c.AddJob(Job{Route{url, "GetMultiFurther"}, image})
		}
	})
}

func (c *Client) GetMultiFurther(j Job) {
	doc, _ := c.GetUrl(j.Route.Url)

	doc.Find("img").Each(func(_ int, s *goquery.Selection) {
		src, exists := s.Attr("src")
		if !exists {
			log.Println("Attribute 'src' dose not exist")
		} else {
			image := j.Data.(Image)
			image.Path = src
			image.Referer = j.Route.Url

			c.AddJob(Job{Route{Url: src, Method: "Download"}, image})
		}
	})
}

func (c *Client) Download(j Job) {
	image := j.Data.(Image)

	dirname := path.Join(dir, image.Format(dirformat, true))
	filename := path.Join(dirname, image.Format(fileformat, false))

	os.MkdirAll(dirname, 0777)
	file, err := os.Create(filename)
	if err != nil {
		log.Println(err)
	}
	defer file.Close()

	b := backoff.NewExponentialBackOff()
	t := backoff.NewTicker(b)

	req, _ := http.NewRequest("GET", j.Route.Url, nil)
	req.Header.Set("Accept", "image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "Keep-Alive")
	req.Header.Set("Host", "i2.pixiv.net")
	req.Header.Set("Referer", image.Referer)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_10_3) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/42.0.2311.135 Safari/537.36")

	var res *http.Response
	for range t.C {
		if res, err = c.Do(req); err != nil {
			log.Println("c.Do", "Error:", err, "Job:", j, "will retry...")
			continue
		}

		if _, err = io.Copy(file, res.Body); err != nil {
			log.Println("io.Copy", "Error:", err, "Job:", j, "will retry...")
			continue
		}

		t.Stop()
		break
	}
	defer res.Body.Close()
}
