package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tevino/tcp-shaker"
)

var (
	urlRE      = regexp.MustCompile(`https?:\/\/(www\.)?[-a-zA-Z0-9@:%._\+~#=]{1,256}\.[a-zA-Z0-9]{1,6}\b([-a-zA-Z0-9!@:%_\+.~#?&\/\/=]*)`)
	skipStatus = flag.String("a", "", "-a 500,400")
	timeout    = flag.Duration("t", 5*time.Second, "-t 10s or -t 1h")
	whitelist  = flag.String("w", "", "-w server1.com,server2.com")
	size       = flag.Int("s", 50, "-s 50")
)

var (
	errorColor    = "\033[1;31m%d\033[0m"
	errorStrColor = "\033[1;31m%s\033[0m"
	okColor       = "\033[1;32m%d\033[0m"
	debugColor    = "\033[1;36m%d\033[0m"
)

type response struct {
	URL      string
	Response int
	Err      error
}

func main() {
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		log.Fatal("filename is required")
	}

	// read file
	file, err := os.ReadFile(args[0])
	if err != nil {
		log.Fatalf("error on reading file: %v", err)
	}

	// validate skipStatus
	var skipped []int
	if len(*skipStatus) > 0 {
		splitted := strings.Split(*skipStatus, ",")
		for _, item := range splitted {
			val, err := strconv.Atoi(item)
			if err != nil {
				log.Fatalf("could not parse skip status value: %v \n", err)
			}
			skipped = append(skipped, val)
		}
	}

	// validate whitelist
	var whitelisted []string
	if len(*whitelist) > 0 {
		whitelisted = strings.Split(*whitelist, ",")
	}

	matches := urlRE.FindAllString(string(file), -1)
	c := tcp.NewChecker()

	ctx, stopChecker := context.WithCancel(context.Background())
	defer stopChecker()
	go func() {
		if err := c.CheckingLoop(ctx); err != nil {
			fmt.Println("checking loop stopped due to fatal error: ", err)
		}
	}()

	results := make(chan *response)

	// producer
	counter := 0
	for _, url := range matches {
		u := url
		if isInStr(url, whitelisted) {
			continue
		}
		counter++
		<-c.WaitReady()
		go worker2(u, results, c)
	}
	fmt.Printf("Found %d URIs\n", len(matches))

	totalErrors := 0
	for counter > 0 {
		resp := <-results
		counter--
		if resp.Err != nil && resp.Response != 200 {
			fmt.Printf("[%s] %s\n", fmt.Sprintf(errorStrColor, "ERROR"), resp.Err.Error())
			totalErrors++
			continue
		}

		shouldSkipURL := len(skipped) > 0 && isIn(resp.Response, skipped)
		statusColor := okColor
		if resp.Err != nil && !shouldSkipURL {
			statusColor = errorColor
			totalErrors++
		} else if shouldSkipURL {
			statusColor = debugColor
		}

		fmt.Printf("[%s] %s \n", fmt.Sprintf(statusColor, resp.Response), resp.URL)
	}

	if totalErrors > 0 {
		fmt.Printf("Total Errors: %s \n", fmt.Sprintf(errorColor, totalErrors))
		os.Exit(1)
	}
}

func worker2(url string, results chan<- *response, c *tcp.Checker) {
	response := &response{
		URL: url,
	}

	runes := []rune(url)
	if runes[len(runes)-1] == '/' {
		url = string(runes[0 : len(runes)-1])
	}

	timeout := time.Second * 1
	err := c.CheckAddr(url+":80", timeout)

	switch err {
	case tcp.ErrTimeout:
		response.Err = err
	case nil:
		response.Response = 200
	default:
		response.Err = err
	}

	results <- response
}

func isIn(item int, items []int) bool {
	for _, i := range items {
		if i == item {
			return true
		}
	}
	return false
}

func isInStr(item string, items []string) bool {
	for _, i := range items {
		if i == item {
			return true
		}
	}
	return false
}
