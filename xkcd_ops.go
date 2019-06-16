// xkcd_ops.go provides operations for updating, viewing,
// and searching xkcd.com web comic data
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/golang/protobuf/proto"
	"gpl/ch4/exercises/e4.12/xkcd"
)

// Data is used to find the DocID's common to all terms in query
type Data struct {
	Key   string
	Value []int
	Len   int
}

func main() {
	// command-line flags/if statements for choosing function
	update := flag.Bool("u", false, "update index")
	viewIndex := flag.Bool("vi", false, "view inverted index")
	viewData := flag.Bool("vd", false, "view data index")
	search := flag.Bool("s", false, "search index")

	flag.Parse()
	if *update != false {
		updateIndex()
	}
	if *viewIndex != false {
		viewInvertedIndex()
	}
	if *viewData != false {
		viewDataIndex()
	}
	if *search != false {
		err := searchIndex()
		if err != nil {
			fmt.Println(err)
		}
	}
}

// updateIndex updates the index since the most recent file stored
func updateIndex() {
	xkcd.GetIndex() // first run - log.db does not exist
	err := xkcd.GetInfo()
	if err != nil {
		fmt.Printf("failed: %v", err)
	}
}

// viewInvertedIndex displays the inverted index
func viewInvertedIndex() {
	ct := 0
	db, oErr := bolt.Open("xkcd_index.db", 0766, nil)
	if oErr != nil {
		fmt.Printf("db failed to open:\n%s", oErr)
	}
	defer db.Close()

	vErr := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("main"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fmt.Printf("key = '%s'\tvalue = %v\n", k, xkcd.Bstois(v))
			ct++
		}
		return nil
	})

	if vErr != nil {
		fmt.Printf("view op failed: %s\n", vErr)
	}

	fmt.Println("\nTotal entries: %v", ct)
}

// viewDataIndex displays the index of json data stored as protocol buffers
func viewDataIndex() {
	ct := 0
	db, oErr := bolt.Open("xkcd_index.db", 0766, nil)
	if oErr != nil {
		fmt.Printf("db failed to open:\n%s", oErr)
	}
	defer db.Close()

	vErr := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("data"))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			fmt.Printf("key = '%v'\tvalue = %+v\n\n", xkcd.Btoi(k), decodeProto(v))
			ct++
		}
		return nil
	})

	if vErr != nil {
		fmt.Printf("view op failed: %s\n", vErr)
	}

	fmt.Println("\nTotal entries: %v", ct)
}

// searchIndex returns data for all files containing every word in query
func searchIndex() error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter search query: ")

	// Get references for each term in query as user input
	text, _ := reader.ReadString('\n')
	query := strings.Split(text, " ")
	resultMap, err := getRefs(query)
	if err != nil {
		return fmt.Errorf("failed to get results: %v", err)
	}

	// Skip sorting and intersection if only one word in query
	if len(resultMap) == 1 {
		for _, v := range resultMap {
			r := returnData(v)
			for _, s := range r {
				fmt.Printf("Num: %d\nLink: %s\nTitle: %s\nTranscript: %s\n\n",
					s.Num, s.Link, s.Title, s.Transcript)
			}
		}
		return nil
	}

	// Sort lists by smallest to largest
	sorted := sortMap(resultMap)

	// Compare values in each list and find all common values
	// Start by finding the common values in the 2 smallest lists
	// then compare the next list to the previous comparison's intersection
	s1, s2 := sorted[0].Value, sorted[1].Value
	common := intersection(s1, s2)
	for _, v := range sorted[2:] {
		common = intersection(common, v.Value)
	}

	// Get data for the common values
	results := returnData(common)
	fmt.Println("results returned")
	for _, v := range results {
		fmt.Printf("Num: %d\nTitle: %s\nTranscript: %s\nLink: %s\n\n",
			v.Num, v.Title, v.Transcript, v.Link)
	}
	return nil
}

// getRefs finds the references for each term in query
func getRefs(q []string) (map[string][]int, error) {
	var resultMap = make(map[string][]int)
	var result []int
	db, oErr := bolt.Open("xkcd_index.db", 0766, nil)
	if oErr != nil {
		fmt.Printf("db failed to open:\n%s", oErr)
	}
	defer db.Close()

	// Get index list for each term in query - use map
	for _, v := range q {
		vErr := db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("main"))
			v = strings.TrimSpace(v)
			result = xkcd.Bstois(b.Get([]byte(v)))
			return nil
		})

		if vErr != nil {
			return nil, fmt.Errorf("view op failed: %s", vErr)
		}
		resultMap[v] = result
	}
	return resultMap, nil
}

// sortMap converts k:v pairs to struct, adds and sorts by len(v)
func sortMap(m map[string][]int) []Data {
	// []Data represnts inverted index
	var ss []Data
	for k, v := range m {
		ss = append(ss, Data{k, v, len(v)}) // term, refs, len
	}

	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Len < ss[j].Len
	})

	return ss
}

// intersection returns the intersection of two integer slices
func intersection(s1, s2 []int) (c []int) {
	checkMap := map[int]bool{}
	for _, v := range s1 {
		checkMap[v] = true
	}
	for _, v := range s2 {
		if v > s1[len(s1)-1] {
			break // break if v > largest number in smaller slice
		}
		if _, ok := checkMap[v]; ok {
			c = append(c, v)
		}
	}
	return
}

// returnData retreives the data for each DocID common to all slices in query
func returnData(c []int) []xkcd.LogData {
	var results []xkcd.LogData
	db, oErr := bolt.Open("xkcd_index.db", 0766, nil)
	if oErr != nil {
		fmt.Printf("db failed to open:\n%s", oErr)
	}
	defer db.Close()

	for _, v := range c {
		vErr := db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte("data"))
			data := decodeProto(b.Get([]byte(xkcd.Itob(v))))
			results = append(results, data)
			return nil
		})

		if vErr != nil {
			fmt.Printf("view op failed: %s\n", vErr)
		}
	}
	return results
}

// decodeProto decodes protocol buffers stored in database to structs
func decodeProto(pb []byte) xkcd.LogData {
	o := &xkcd.LogDataStruct{}
	err := proto.Unmarshal(pb, o)
	if err != nil {
		log.Fatalf("unmarshal failed: %v\n", err)
	}

	entry := xkcd.LogData{o.GetMonth(), o.GetNum(), o.GetLink(), o.GetYear(),
		o.GetNews(), o.GetSafeTitle(), o.GetTranscript(), o.GetAlt(), o.GetImg(),
		o.GetTitle(), o.GetDay()}

	return entry
}
