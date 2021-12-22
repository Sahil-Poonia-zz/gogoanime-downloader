package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"golang.org/x/net/html"
)

const (
	UserAgent    = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.110 Safari/537.36"
	Referer      = "https://gogoplay1.com/"
	BrightRed    = "\u001b[31;1m"
	BrightGreen  = "\u001b[32;1m"
	BrightYellow = "\u001b[33;1m"
	BrightWhite  = "\u001b[37;1m"
	Reset        = "\u001b[0m"
)

func GetRequest(targetURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("GetRequest(): %v", err)
	}
	req.Header.Add("User-Agent", UserAgent)
	req.Header.Add("Referer", Referer)
	req.Header.Add("Accept", "*/*")

	fetchErr := fmt.Errorf("")
	for tries := 0; tries < 3; tries++ {
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			return resp, nil
		}

		fetchErr = err
		fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n\tRetrying in %v2%v seconds.\n", BrightRed, Reset, targetURL, BrightWhite, Reset)
		time.Sleep(time.Second * 2)
	}

	return nil, fmt.Errorf("GetRequest(): %v", fetchErr)
}

func dowPgLinkParser(n *html.Node, resultant *string) {
	if n.Type == html.ElementNode && n.Data == "li" && n.FirstChild != nil && n.FirstChild.Data == "a" {
		for _, liAttr := range n.Attr {
			if liAttr.Key == "class" && liAttr.Val == "dowloads" {
				for _, aAttr := range n.FirstChild.Attr {
					if aAttr.Key == "href" {
						*resultant = aAttr.Val
					}
				}
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		dowPgLinkParser(c, resultant)
	}
}

func dowLinkParser(n *html.Node, resultant map[string]string) {
	if n.Type == html.ElementNode && n.Data == "a" {
		for _, attr := range n.Attr {
			if attr.Key == "download" {
				var quality string
				for _, char := range n.FirstChild.Data {
					if string(char) == "P" {
						break
					}
					charNum, err := strconv.ParseInt(string(char), 10, 64)
					if err != nil {
						continue
					}
					quality += strconv.Itoa(int(charNum))
				}
				resultant[quality] = n.Attr[0].Val
			}
		}
	}

	for c := n.FirstChild; c != nil; c = c.NextSibling {
		dowLinkParser(c, resultant)
	}
}

func main() {
	urlPtr := flag.String("url", "", "URL of episode page but without episode number.\n(ex: 'https://gogoanime.wiki/dragon-ball-super-dub-episode-' Note that link does not contain\nepisode number at the end )")
	fromPtr := flag.Int("from", 1, "episode number you want to download from.")
	toPtr := flag.Int("to", 1, "episode number you want to download to.")
	flag.Parse()

	switch {
	case *urlPtr == "":
		log.Fatal(fmt.Sprintf("[%vERROR%v] URL is required.\n", BrightRed, Reset))
	case *fromPtr < 1, *toPtr < *fromPtr:
		log.Fatal(fmt.Sprintf("[%vERROR%v] From: %v and To: %v values don't make sense.\n", BrightRed, Reset, *fromPtr, *toPtr))
	}

	logFile, err := os.OpenFile("gogoanime-errors.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		log.Fatal(fmt.Sprintf("[%vFATAL%v] Failed to open/create log file.\n%v", BrightRed, Reset, err))
	}
	defer logFile.Close()
	log.SetOutput(logFile)
	log.SetPrefix("main(): ")

	for i := *fromPtr; i <= *toPtr; i++ {
		episodeLink := *urlPtr + strconv.Itoa(i)

		fmt.Printf("[%v+%v] EPISODE %v\n", BrightGreen, Reset, i)
		body, err := GetRequest(episodeLink)
		if err != nil {
			log.Print(err)
			fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n", BrightRed, Reset, episodeLink)
			continue
		}
		defer body.Body.Close()
		fmt.Printf("\t[%v*%v] Connection to episode page succeeded.\n", BrightGreen, Reset)

		bodyDoc, _ := html.Parse(body.Body)
		var dowPgLink string
		dowPgLinkParser(bodyDoc, &dowPgLink)

		dowPgBody, err := GetRequest(dowPgLink)
		if err != nil {
			log.Print(err)
			fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n", BrightRed, Reset, dowPgLink)
			continue
		}
		defer dowPgBody.Body.Close()
		fmt.Printf("\t[%v*%v] Connection to download page succeeded.\n", BrightGreen, Reset)

		dowPgBodyDoc, _ := html.Parse(dowPgBody.Body)
		dowLinks := make(map[string]string)
		dowLinkParser(dowPgBodyDoc, dowLinks)

		var dowLink, episodeQuality string
		isKey := func(key string) bool { _, exists := dowLinks[key]; return exists }
		switch {
		case isKey("1080"):
			episodeQuality = "1080P"
			dowLink = dowLinks["1080"]
		case isKey("720"):
			episodeQuality = "720P"
			dowLink = dowLinks["720"]
		case isKey("480"):
			episodeQuality = "480P"
			dowLink = dowLinks["480"]
		case isKey("360"):
			episodeQuality = "360P"
			dowLink = dowLinks["360"]
		}

		fileResp, err := GetRequest(dowLink)
		if err != nil {
			log.Print(err)
			fmt.Printf("[%vERROR%v] Failed to fetch URL: %v\n", BrightRed, Reset, dowLink)
			continue
		}
		defer fileResp.Body.Close()

		fmt.Printf("\t[%v*%v] Episode Quality: %v\n", BrightGreen, Reset, episodeQuality)
		fmt.Printf("\t[%v*%v] Episode Size: %v MB\n", BrightGreen, Reset, (fileResp.ContentLength / (1024 * 1024)))

		episodeFile, err := os.OpenFile(fmt.Sprintf("%v.mp4", i), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Print(err)
			log.Fatal(fmt.Sprintf("[%vFATAL%v] Failed to create episode file.\n%v", BrightRed, Reset, err))
		}
		defer episodeFile.Close()

		fmt.Printf("\t[%vDOWNLOADING%v] ...\n", BrightYellow, Reset)
		_, err = io.Copy(episodeFile, fileResp.Body)
		if err != nil {
			log.Print(err)
			fmt.Printf("\t[%vERROR%v] Failed to download episode :(\n", BrightRed, Reset)
			continue
		}
		fmt.Printf("\t[%vDONE%v] Episode download succeeded.\n", BrightGreen, Reset)
	}
}
