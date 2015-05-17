package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"reflect"
	"sync"
)

const (
	PixivHost   = "http://www.pixiv.net"
	AuthorUrl   = PixivHost + "/member_illust.php"
	IllustUrl   = PixivHost + "/member_illust.php"
	IllustsSize = 20
)

var (
	memberId    string
	user        string
	pass        string
	fileformat  string
	dirformat   string
	dir         string
	workerCount int
)

func init() {
	// Get current work directory
	d, _ := os.Getwd()

	flag.StringVar(&user, "user", "", "the user name to login")
	flag.StringVar(&pass, "pass", "", "the password of the login user")
	flag.StringVar(&fileformat, "file-format", "pixiv-{{Illust.Id}}-{{Illust.Name}}-{{Author.Name}}-{{Image.Id}}", "the format of the image name")
	flag.StringVar(&dirformat, "dir-format", "{{Author.Name}}-{{Author.Id}}", "the format of the directory name")
	flag.StringVar(&dir, "dir", d, "the directory to save the images")
	flag.IntVar(&workerCount, "worker-count", 10, "the max count of concurreny working jobs")
	flag.Parse()

	if flag.NArg() < 1 || user == "" || pass == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <id>\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	memberId = flag.Arg(0)
}

func main() {
	queue := make(chan Job, 20)
	workers := make(chan struct{}, workerCount)
	wg := new(sync.WaitGroup)

	wait := func() {
		go func() {
			wg.Wait()
			close(queue)
		}()
	}

	addJob := func(j Job) {
		wg.Add(1)
		queue <- j
	}

	cookieJar, _ := cookiejar.New(nil)
	c := Client{
		&http.Client{
			Jar: cookieJar,
		},
		reflect.ValueOf(addJob),
	}

	c.Login(user, pass)

	addJob(Job{Route: Route{getUrl(), "GetAuthor"}})

	once := new(sync.Once)

	for j := range queue {
		once.Do(wait)
		go func(j Job) {
			workers <- struct{}{}
			defer func() {
				<-workers
			}()
			defer wg.Done()
			defer log.Println("Job Done\t", j)

			log.Println("Start Job\t", j)
			reflect.ValueOf(&c).MethodByName(j.Route.Method).
				Call([]reflect.Value{reflect.ValueOf(j)})
		}(j)
	}

	log.Println("Complete!")
}

func getUrl() string {
	u, _ := url.Parse(AuthorUrl)
	q := u.Query()
	q.Set("id", memberId)
	u.RawQuery = q.Encode()
	return u.String()
}
