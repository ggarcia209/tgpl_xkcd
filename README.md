# tgpl_xkcd
Files for building search index of xkcd.com web comics - from Exercise 4.12 in The Go Programming Language

Version 1.0

*** Prerequisites ***

Must have Protocol Buffers and BoltDb installed.

*** Application Overview ***

This application is composed of four files, 'xkcd_data.go', 'xkcd_ops.go', 'logData.pb.go', and 'logData.proto'. This application builds a searchable index from the JSON metadata of every web comic on xkcd.com. This is a fairly simple search engine and does not yet implement more advanced features such as stemming, normalization, and positional indexing. 

Running the program for the first time will create the 'comic_log.txt', 'xkcd_index.db', and 'log.db' files in the main/parent directory containing 'xkcd_ops.go'. 'xkcd_data.go', 'logData.pb.go', and 'logData.proto' are stored in the child directory, 'xkcd_data'. 

Ex: store 'xkcd_ops.go' in 'go/src/xkcd' 
    store 'xkcd_data.go', 'logData.pb.go', and 'logData.proto' in 'go/src/xkcd/xkcd_data'

*** xkcd_data.go Overview ***

The first file, 'xkcd_data.go' is used to download, format, and store the data for each comic in a boltDB database on disk. Additionally, it builds an inverted index for every term in each comic and writes the raw data for each comic to a .txt log. The inverted index is stored in the same database as the comic data under a different bucket. 

*** Building the Indices ***

The data is first decoded from JSON to the 'MapData' struct. The inverted index is built by mapping the 'Index' (top-level var for DocID) of each comic to each term (key) in the 'Num' (DocID), 'Year', 'Transript', 'Alt', and 'Title' fields contained in the comic. The 'Index' values for each term are appended to a slice. The slices will always be ordered and contain unique integer values. The 'LogData' struct (complete metadata) for each comic is mapped to the 'Index' of each comic. 

Once the in-memory maps are updated for each comic, the raw data is appended to the 'comic_log.txt' file. Once all http responses up to, including the most recent comic are processed, the maps are stored in the database. The inverted index key/value pairs are converted to byte slices and stored, while the data map values are encoded and stored as protocol buffers. The final 'Index' value is stored in a seperate database, 'log.db', which allows for constant look-up time. On subsequent database updates, the previous 'Index' is overwritten in 'log.db'. Only one key/value pair, '"index": Index' is ever stored. 


*** xkcd_ops.go Overview ***

'xkcd_ops.go' provides operations for updating, viewing and searching the data. The data is updated with the 'u' flag, viewed with either 'vi' or 'vd' flags (view inverted index or view data), and searched with the 's' flag.

*** Creating/Updating Data ***

The program has been designed to allow regular updates of the data without overwriting any of the existing data. To do this, the latest 'Index' is retrieved from 'log.db' before the data is downloaded, processed, and stored. If 'log.db' doesn't exist (first execution), it is created and the 'Index' is set to 1. Subsequent executions of the program pick up where the last execution left off. The .txt log is appended to, the inverted index slices are appended to, and new 'Index'/'LogData' k/v pairs are added to the database. 

*** Viewing Data ***

Both the complete inverted index and 'LogData' index can be viewed seperately using the flags described above. The complete datasets will be printed along with the total number of entries in each set. 

*** Searching Data ***

The search function is implemented by first gathering a user-input query. Version 1.0 will not return any results if punctuation is used in the query. Once the query has been read in, the lists (int slices) of the corresponding indices are returned for each term. The lists are then sorted by size, smallest to largest. Once they are sorted, the intersection (common values) are found for every list. This is accomplished by first finding the intersection of the two smallest lists, then finding the intersection of the next largest list and the common values of the preceding comparison. The latter step is repeated for the remainder of the index lists. 

After the common values have been found, the 'Num', 'Link', 'Title', and 'Transcript' data for each index in the common values list are decoded from the protocol buffers stored in the on disk database and displayed to the user. As stated previously, this a fairly simple and limited search engine. The results returned simply contain every word in the query. Future versions may implement features like searching by specific fields, such as searching for all comics from a given month, stemming, positional indexing, and normalization.

*** Protocol Buffers Files ***

'logData.pb.go', and 'logData.proto' are the protocol buffers files required to implement protocol buffers and store data to the database in this format. 

*** Performance ***

Building the index from scratch (~2160 JSON files, ~22,000 terms, ~2160 data structs as of 6/15/19) uses ~30MB RAM, ~10% (avg) of a 2.7 GHz Intel Core i7 processor, and takes about 2-3 minutes to complete. Viewing and searching the complete datasets is near instantaneous and takes < 1 seconds to return data for the largest result sets. Performance data gathered from the MacOS Activity Monitor. 

*** Other Limitations ***
* Subsequent executions panic if first execution fails to log Index.
	- 'log.db' file created with nil pointer reference.
* Rerunning program if storeIndexMap or logIndexVar fails  will create duplicate entries in 'comic_log.txt' because it is append-only (after successfully executing program at least once, see above).
* Inputting a blank query opens & closes the database and ends the process without returning any results or error message.
  
*** Future Objectives ***
* Create atomicity in each execution without deleting previous data successfully stored. 
  - specifically referring to 'comic_log.txt' file. See above regarding duplicate entries. Data stored in 'xkcd_index.db' should not be affected if program fails - BoltDB uses transactions and a write lock while transactions are open.
  - This should not be an issue in the current version (1.0). Program has yet to fail during testing. 
* Create function to restore from 'comic_log.txt' in the event of failure, specifically regarding the 'Index' failing to be stored in 'log.db'. 
* Implement advanced search features such as stemming, normalization, positional indexing, ranking by frequency, and searching by specific fields. 
