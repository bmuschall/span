package main

import (
	"bufio"

	"github.com/miku/span"
	"github.com/miku/span/crossref"
	"github.com/miku/span/holdings"

	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"
)

// Options for worker.
type Options struct {
	Holdings     holdings.IsilIssnHolding
	IgnoreErrors bool
	Verbose      bool
	NumWorkers   int
	BatchSize    int
}

// BatchedLDJProcessor will send batches of line to the workers.
func BatchedLDJProcessor(filename string, options Options) error {
	ff, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	defer ff.Close()
	reader := bufio.NewReader(ff)

	batches := make(chan []string)
	docs := make(chan []byte)
	done := make(chan bool)

	go ByteSliceCollector(docs, done)

	var wg sync.WaitGroup
	for i := 0; i < options.NumWorkers; i++ {
		wg.Add(1)
		go BatchedLDJWorker(batches, docs, options, &wg)
	}

	i := 1
	batch := make([]string, options.BatchSize)
	for {
		line, err := reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		batch = append(batch, line)
		if i == options.BatchSize {
			batches <- batch
			batch = batch[:0]
			i = 1
		}
		i++
	}
	batches <- batch
	close(batches)
	wg.Wait()
	close(docs)
	<-done
	return nil
}

// BatchedLDJWorker receives batches of strings, parses, transforms and serializes them.
func BatchedLDJWorker(batches chan []string, out chan []byte, options Options, wg *sync.WaitGroup) {
	defer wg.Done()
	for batch := range batches {
		for _, line := range batch {
			doc := new(crossref.Document)
			json.Unmarshal([]byte(line), &doc)
			schema, err := doc.ToSolrSchema()
			if err != nil {
				if options.Verbose {
					log.Println(err)
				}
				if options.IgnoreErrors {
					continue
				}
			}
			schema.SetTags(doc.Institutions(options.Holdings))
			b, err := json.Marshal(schema)
			if err != nil {
				log.Fatal(err)
			}
			out <- b
		}
	}
}

// Collector collects docs and writes them out to stdout.
func ByteSliceCollector(docs chan []byte, done chan bool) {
	f := bufio.NewWriter(os.Stdout)
	defer f.Flush()
	for b := range docs {
		f.Write(b)
		f.Write([]byte("\n"))
	}
	done <- true
}

func main() {
	batchSize := flag.Int("b", 25000, "batch size")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	numWorkers := flag.Int("w", runtime.NumCPU(), "workers")
	version := flag.Bool("v", false, "prints current program version")

	hspec := flag.String("hspec", "", "ISIL PATH pairs")
	members := flag.String("members", "", "path to LDJ file, one member per line")

	ignoreErrors := flag.Bool("ignore", false, "skip broken input record")
	verbose := flag.Bool("verbose", false, "print debug messages")

	PrintUsage := func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS] FILE\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.Parse()

	runtime.GOMAXPROCS(*numWorkers)

	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if *version {
		fmt.Println(span.Version)
		os.Exit(0)
	}

	options := Options{
		Holdings:               make(holdings.IsilIssnHolding),
		IgnoreErrors:           *ignoreErrors,
		Verbose:                *verbose,
		AllowEmptyInstitutions: *allowEmptyInstitutions,
		NumWorkers:             *numWorkers,
		BatchSize:              *batchSize,
	}

	if *hspec != "" {
		pathmap, err := span.ParseHoldingSpec(*hspec)
		if err != nil {
			log.Fatal(err)
		}
		for isil, path := range pathmap {
			file, err := os.Open(path)
			if err != nil {
				log.Fatal(err)
			}
			defer file.Close()
			options.Holdings[isil] = holdings.HoldingsMap(bufio.NewReader(file))
		}
	}

	if flag.NArg() == 0 {
		PrintUsage()
		os.Exit(1)
	}

	if *members != "" {
		err := crossref.PopulateMemberNameCache(*members)
		if err != nil {
			log.Fatal(err)
		}
	}

	err := BatchedLDJProcessor(flag.Arg(0), options)
	if err != nil {
		log.Fatal(err)
	}
}
