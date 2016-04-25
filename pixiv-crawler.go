package main

import (
	"flag"
	"fmt"
	"github.com/cenkalti/backoff"
	"log"
	"net/http"
	"net/http/cookiejar"
	"os"
	"reflect"
	"sync"
)

const (
	PIXIV_HOST  = "http://www.pixiv.net"
	LOGIN_URL   = "https://www.pixiv.net/login.php"
	AUTHOR_URL  = PIXIV_HOST + "/member_illust.php"
	ILLUST_URL  = PIXIV_HOST + "/member_illust.php"
	ILLUST_SIZE = 20
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
	queue := make(chan Job, 20)                 // queue of jobs
	workers := make(chan struct{}, workerCount) // queue of workers
	var wg sync.WaitGroup

	wait := func() {
		go func() {
			// Wait until all jobs has done
			wg.Wait()
			close(queue)
		}()
	}

	addJob := func(j Job) {
		wg.Add(1)
		queue <- j
	}

	// Initialize the crawler
	cookieJar, _ := cookiejar.New(nil)
	c := Crawler{
		&http.Client{
			Jar: cookieJar,
		},
		reflect.ValueOf(addJob),
	}

	addJob(Job{Route{LOGIN_URL, "Login"}, Certification{user, pass}})

	var once sync.Once

	for j := range queue {
		once.Do(wait)
		go func(j Job) {
			// Recover error, just fatal
			defer func() {
				if e := recover(); e != nil {
					log.Fatalln(e)
				}
			}()

			// Block if the workers is fully loaded
			workers <- struct{}{}
			defer func() {
				<-workers
			}()

			defer wg.Done()

			log.Println("Start Job ", j)

			b := backoff.NewExponentialBackOff()
			t := backoff.NewTicker(b)

			method := reflect.ValueOf(&c).MethodByName(j.Route.Method)

			// Call special method of crawler on the job
			// If any error return, retry
			var err interface{}
			for range t.C {
				returns := method.Call([]reflect.Value{reflect.ValueOf(j)})

				if err = returns[0].Interface(); err != nil {
					log.Println(err, "will retry...")
					continue
				}

				t.Stop()
				break
			}

			if err != nil {
				log.Println("Job Fail ", err.(error), j)
			} else {
				log.Println("Job Done ", j)
			}
		}(j)
	}

	log.Println("Complete!")
}
