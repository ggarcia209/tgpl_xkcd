// Package xkcd contains objects and functions for retreiving,
// storing and indexing JSON data from xkcd.com web comics.
package xkcd

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/boltdb/bolt"
	proto "github.com/golang/protobuf/proto"
)

// XKCDURL is the server domain name.
const XKCDURL = "https://xkcd.com/"

// URL is the url for each comic (ex: 'https://xkcd.com/209')
var URL string

// Index tracks the number of entries created and enables subsequent
// executions of program to pick up where last execution left off.
var Index int

// IndexMap is the inverted index of each term and the docs they appear in.
var IndexMap = make(map[string][]int)

// DataMap stores the Index and LogData of each json file as key: value pairs
var DataMap = make(map[int]LogData)

// Entry formats JSON data for storing to log file.
type Entry struct {
	Index int
	Data  *LogData
}

// LogData stores unmarshalled JSON data and is used to
// encode/decode data as protocol buffers for db storage
type LogData struct {
	Month      string
	Num        int32
	Link       string
	Year       string
	News       string `json:"omitempty"`
	SafeTitle  string `json:"omitempty"`
	Transcript string
	Alt        string
	Img        string
	Title      string
	Day        string
}

// MapData stores/formats unmarshalled JSON data to be mapped to index
// Img url, Month & Day numbers are not stored
// Search by month/day functionality may be added in future version
type MapData struct {
	Num        int
	Year       string
	News       string `json:"omitempty"`
	SafeTitle  string `json:"omitempty"`
	Transcript string
	Alt        string
	Title      string
}

// GetIndex updates 'Index' var in memory from persistent value stored in 'log.db'
// GetIndex allows for constant look up time vs. scanning over each existing entry in linear time
func GetIndex() {
	if _, err := os.Stat("log.db"); os.IsNotExist(err) {
		// 'log.db' does not exist
		fmt.Print("log.db not found\n")
		Index = 1
		fmt.Printf("index at start = %v\n", Index)
	} else {
		fmt.Print("log.db found\n")
		Index = viewLogDb()
		fmt.Printf("index at start = %v\n", Index)
	}
	return
}

// GetInfo retrieves JSON info for each comic's webpage,
// maps each term in each response to in-memory inverted index,
// and writes unmarshalled data to file as an append-only log.
func GetInfo() error {
	// Open or create file as append-only
	f, err := os.OpenFile("comic_log.txt", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0766)
	if err != nil {
		return fmt.Errorf("failed to open comic_log.txt: %v", err)
	}

	// Get JSON data from each comic's URL
	fmt.Printf("downloading and mapping JSON info...\n")
	for i := Index; i > 0; i++ { // increment +1 for next url
		if i == 404 { // skip special case - http 404 error page
			Index++
			continue
		}

		jsonURL := XKCDURL + strconv.Itoa(i) + "/info.0.json"
		URL = XKCDURL + strconv.Itoa(i)
		resp, err := http.Get(jsonURL) // "https://xkcd.com/i/info.0.json"
		if err != nil {
			resp.Body.Close()
			return fmt.Errorf("request failed: %s\n http responses processed: %v", err, Index)
		}
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			resp.Body.Close()
			return fmt.Errorf("request failed: %s\n http responses processed: %v", resp.Status, Index)
		}
		if resp.StatusCode == http.StatusNotFound { // Break loop after most recent comic
			break
		}

		// Convert JSON info in HTTP response to byte array
		respInfo, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		// Map terms and data in memory & write raw data to log file
		mapTerms(formatEntry(respInfo))
		mapData(respInfo, Index)
		wErr := writeOutput(f, respInfo)
		if wErr != nil {
			return fmt.Errorf("Write to comic_log.txt failed:\n%v", err)
		}

		fmt.Printf("file processed: %v\n", (Index))
		Index++ // increment index/DocID for every http response processed

	}
	f.Close()
	fmt.Printf("in memory map created\ntotal files processed: %v\n", (Index - 1))

	// Store IndexMap, DataMap and Index on disk
	sErr := storeIndexMap(IndexMap)
	if sErr != nil {
		return fmt.Errorf("StoreIndexMap failed: %v", sErr)
	}
	fmt.Println("inverted index saved to disk")

	sErr = storeMapData(DataMap)
	if sErr != nil {
		return fmt.Errorf("StoreMapData failed: %v", sErr)
	}
	fmt.Println("data map saved to disk")

	lErr := logIndexVar(Index)
	if lErr != nil {
		return fmt.Errorf("StoreIndexMap failed: %v", sErr)
	}
	fmt.Println("index logged on disk for next execution")

	return nil
}

// viewLogDb returns the 'Index' value (# of docs processed)
// logged at end of the last execution of the program
func viewLogDb() int {
	var index int
	db, oErr := bolt.Open("log.db", 0766, nil)
	if oErr != nil {
		fmt.Printf("db failed to open:\n%s", oErr)
	}
	defer db.Close()

	vErr := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("log"))
		index = Btoi(b.Get([]byte("index")))
		return nil
	})
	if vErr != nil {
		fmt.Printf("view op failed: %s\n", vErr)
	}
	return index
}

// writeOutput unmashalls data from each http reseponse to Info struct
// and writes to end of 'comic_log.txt' file
func writeOutput(f *os.File, respInfo []byte) error {
	// Unmarshal JSON data to Info struct
	var comicData *LogData
	if err := json.Unmarshal(respInfo, &comicData); err != nil {
		return fmt.Errorf("JSON unmarshalling failed: %s\n files written: %v", err, Index)
	}

	// Write unmarshalled struct to output file as byte array of string
	w := bufio.NewWriter(f)
	e := Entry{Index, comicData}
	w.Write([]byte(fmt.Sprintf("%v:\t%+v¶\n\n", e.Index, e.Data)))
	w.Flush()

	return nil
}

// formatEntry formats JSON data from http response to be parsed for indexing
func formatEntry(data []byte) []byte {
	// unmarshall data to Info struct and format w/o field names
	var mapData *MapData
	if err := json.Unmarshal(data, &mapData); err != nil {
		fmt.Printf("JSON unmarshalling failed: %s\n files written: %v", err, Index)
	}
	s := fmt.Sprintf("%v", mapData) // was e.Data

	// remove & replace non-alpha-numeric characters and lowercase text
	reg, err := regexp.Compile("[^a-zA-Z0-9]+") // removes all non alpha-numeric characters
	if err != nil {
		log.Fatal(err)
	}
	rmNewln := strings.Replace(s, "\n", "    ", -1)  // replace terms joined by \n with 4 spaces (ex: "boy\nThey" -> "boy", "They")
	rmApost := strings.Replace(rmNewln, "'", "", -1) // don't split contractions (ex: 'can't' !-> "can", "t")
	rmComma := strings.Replace(rmApost, ",", "", -1) // don't split numerical values > 999 (ex: 20,000 !-> 20 000)
	lwr := strings.ToLower(rmComma)
	formatted := []byte(reg.ReplaceAllString(lwr, " "))

	return formatted
}

// mapTerms creates an inverted index by mapping each term in each response
// from xkcd.com to the indexes (DocID) of the documents containing it
func mapTerms(data []byte) map[string][]int {
	s := bufio.NewScanner(bytes.NewReader(data))
	s.Split(bufio.ScanWords)
	for s.Scan() {
		IndexMap[s.Text()] = appendIfUnique(IndexMap[s.Text()], Index)
	}
	return IndexMap
}

// mapData creates db index of data mapped to the index of each file
func mapData(data []byte, i int) map[int]LogData {
	var dataMapFields *LogData
	if err := json.Unmarshal(data, &dataMapFields); err != nil {
		fmt.Printf("JSON unmarshalling failed: %s\n files written: %v", err, Index)
	}
	dataMapFields.Link = URL // 'Link' field is empty in json http response
	DataMap[i] = *dataMapFields

	return DataMap
}

// Uses map to check if DocID is unique
func appendIfUnique(s []int, i int) []int {
	imap := make(map[int]bool)
	for _, v := range s {
		imap[v] = true
	}
	if !imap[i] {
		return append(s, i)
	}
	return s
}

// storeIndexMap stores & updates the inverted index in 'xkcd_index.db' file
func storeIndexMap(m map[string][]int) error {
	// open/create db
	db, err := bolt.Open("xkcd_index.db", 0766, nil)
	if err != nil {
		log.Fatalf("could not open:\n%v", err)
	}
	defer db.Close()

	// store values and appends to existing keys
	var i int
	uErr := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("main"))
		if err != nil {
			return fmt.Errorf("create 'main' bucket failed:\n%s", err)
		}

		for k, v := range m {
			new := append(b.Get([]byte(k)), Istobs(v)...)
			err := b.Put([]byte(k), new) // must overwrite old data by appending new to result of b.Get()
			if err != nil {
				return fmt.Errorf("put failed:\n%s", err)
			}
			i++
		}
		return nil
	})

	if uErr != nil {
		return fmt.Errorf("update transaction failed:\n%s", uErr)
	}
	fmt.Printf("entries stored in 'main': %v\n", i)

	return nil
}

// storeMapData stores & updates LogData as protobuf mapped to index in 'xkcd_index.db' file
func storeMapData(m map[int]LogData) error {
	// open db
	db, err := bolt.Open("xkcd_index.db", 0766, nil)
	if err != nil {
		log.Fatalf("could not open:\n%v", err)
	}
	defer db.Close()

	// map LogData struct to each index
	var i int
	uErr := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("data"))
		if err != nil {
			return fmt.Errorf("create 'data' bucket failed:\n%s", err)
		}
		for k, v := range m {
			err := b.Put(Itob(k), convToProto(v)) // must overwrite old data by appending new to result of b.Get()
			if err != nil {
				return fmt.Errorf("put failed:\n%s", err)
			}
			i++
		}
		return nil
	})

	if uErr != nil {
		return fmt.Errorf("update transaction failed:\n%s", uErr)
	}
	fmt.Printf("entries stored in 'data': %v\n", i)

	return nil
}

// convToProto encodes LogData structs as protocol buffers
func convToProto(d LogData) []byte {
	entry := &LogDataStruct{
		Month:      d.Month,
		Num:        d.Num,
		Link:       d.Link,
		Year:       d.Year,
		News:       d.News,
		SafeTitle:  d.SafeTitle,
		Transcript: d.Transcript,
		Alt:        d.Alt,
		Img:        d.Img,
		Title:      d.Title,
		Day:        d.Day,
	}
	data, err := proto.Marshal(entry)
	if err != nil {
		log.Fatalf("proto marshal failed: %v\n", err)
	}
	return data
}

// logIndexVar logs 'Index' (# of http responses processed) for quick lookup next time program runs
func logIndexVar(i int) error {
	db, err := bolt.Open("log.db", 0766, nil)
	if err != nil {
		log.Fatalf("could not open:\n%v", err)
	}
	defer db.Close()

	uErr := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("log"))
		if err != nil {
			return fmt.Errorf("create 'log' bucket failed:\n%s", err)
		}
		pErr := b.Put([]byte("index"), Itob(i))
		if pErr != nil {
			return fmt.Errorf("index log failed:\n%s", err)
		}
		return nil
	})

	if uErr != nil {
		return fmt.Errorf("log transaction failed:\n%s", err)
	}

	return nil
}

// Itob encodes single int to byte slice for db storage
func Itob(i int) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(i))
	return b
}

// Btoi decodes byte slice representing single
// uint16 to single int for db retrieval
func Btoi(b []byte) int {
	return int(binary.BigEndian.Uint16(b))
}

// Istobs encodes an int slice to byte slice for db storage
func Istobs(s []int) []byte {
	var bs []byte
	for _, v := range s {
		b := make([]byte, 2)
		binary.BigEndian.PutUint16(b, uint16(v))
		bs = append(bs, b...)
	}
	return bs
}

// Bstois decodes a byte slice representing multiple
// uint16's to to an int slice for db retrieval
func Bstois(bs []byte) []int {
	var is []int
	for i := 0; i < len(bs); i += 2 {
		b := bs[i:]
		in := int(binary.BigEndian.Uint16(b))
		is = append(is, in)
	}
	return is
}
