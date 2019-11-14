package main

import (
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"sort"
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
	results  []result
	capacity int
	mux      sync.Mutex
}

func (s *SafeStories) add(res result) {
	s.mux.Lock()
	defer s.mux.Unlock()
	if len(s.results) < s.capacity && res.err == nil && isStoryLink(res.item) {
		s.results = append(s.results, res)
	}
}

func asyncAddStory(id int, idx int, client hn.Client, stories *SafeStories, wg *sync.WaitGroup) {
	defer wg.Done()
	hnItem, err := client.GetItem(id)
	var res result
	if err != nil {
		res = result{idx: idx, err: err}
	} else {
		res = result{idx: idx, item: buildHNItem(hnItem)}
	}
	stories.add(res)
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
		safeStories := SafeStories{capacity: numStories, results: make([]result, 0)}
		var wg sync.WaitGroup
		fmt.Println("sending for processing: ")
		for i := 0; i < numStories; i++ {
			wg.Add(1)
			go asyncAddStory(ids[i], i, client, &safeStories, &wg)
			//fmt.Printf("%v, %v \n", i, ids[i])
		}
		wg.Wait()

		// fmt.Printf("results are: \n")
		// for _, v := range safeStories.results {
		// 	fmt.Printf("%v, %v \n", v.idx, v.ID)
		// }

		sort.Slice(safeStories.results, func(i, j int) bool {
			return safeStories.results[i].idx < safeStories.results[j].idx
		})

		// fmt.Printf("filtered results are: \n")
		// for _, v := range safeStories.results {
		// 	fmt.Printf("%v, %v \n", v.idx, v.ID)
		// }

		var stories []item

		for _, v := range safeStories.results {
			stories = append(stories, v.item)
		}

		data := templateData{
			Stories: stories,
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

func buildHNItem(hnItem hn.Item) item {
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

type result struct {
	item
	idx int
	err error
}

type templateData struct {
	Stories []item
	Time    time.Duration
}
