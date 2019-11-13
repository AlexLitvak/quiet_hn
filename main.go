package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/AlexLitvak/quiet_hn/hn"
)

func main() {
	// parse flags
	var port, numStories int
	flag.IntVar(&port, "port", 3000, "the port to start the web server on")
	flag.IntVar(&numStories, "num_stories", 30, "the number of top stories to display")
	flag.Parse()

	tpl := template.Must(template.ParseFiles("./index.gohtml"))

	http.HandleFunc("/", handler(numStories, tpl))

	// Start the server
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", port), nil))
}

type SafeStories struct {
	stories  []item
	capacity int
	mux      sync.Mutex
}

func (s *SafeStories) add(it item) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if len(s.stories) < s.capacity {
		s.stories = append(s.stories, it)
	}
}

func asyncAddStory(id int, client hn.Client, stories *SafeStories, wg *sync.WaitGroup) {
	defer wg.Done()
	hnItem, _ := client.GetItem(id)
	item := parseHNItem(hnItem)
	if isStoryLink(item) {
		stories.add(item)
	}
}

func handler(numStories int, tpl *template.Template) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		var client hn.Client
		ids, err := client.TopItems()
		if err != nil {
			http.Error(w, "Failed to load top stories", http.StatusInternalServerError)
			return
		}
		safeStories := SafeStories{capacity: numStories, stories: make([]item, 0)}
		var wg sync.WaitGroup
		for _, id := range ids {
			wg.Add(1)
			go asyncAddStory(id, client, &safeStories, &wg)
		}
		wg.Wait()
		data := templateData{
			Stories: safeStories.stories,
			Time:    time.Now().Sub(start),
		}
		err = tpl.Execute(w, data)
		if err != nil {
			http.Error(w, "Failed to process the template", http.StatusInternalServerError)
			return
		}
	})
}

func isStoryLink(item item) bool {
	return item.Type == "story" && item.URL != ""
}

func parseHNItem(hnItem hn.Item) item {
	ret := item{Item: hnItem}
	url, err := url.Parse(ret.URL)
	if err == nil {
		ret.Host = strings.TrimPrefix(url.Hostname(), "www.")
	}
	return ret
}

// item is the same as the hn.Item, but adds the Host field
type item struct {
	hn.Item
	Host string
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
