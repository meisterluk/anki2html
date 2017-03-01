package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/template"
	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// HTMLTemplate defines the basic structure of the HTML file
const HTMLTemplate = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Anki Package Dump: {{.Title}}</title>
    <style type="text/css">
    .filepath { font-family: monospace }
    .generated { font-family: monospace }
    .type { padding: 5px; text-align: center; }
    .flashcards {
      width: 70%;
      min-width: 500px;
      display: flex; flex-flow: column nowrap; justify-content: flex-start; align-content: center;
    }
    .flashcard {
      flex: 1 1 auto;
      display: flex; flex-flow: row nowrap; justify-content: space-around; align-items: stretch; align-content: center;
    }
    .flashcard > * { padding: 10px; margin: 10px; min-height: 200px; }
    .flashcard .delim { line-height: 200px; }
    .flashcard .front { width: 40%; box-shadow: #FAA 0px 0px 10px; }
    .flashcard .back { width: 40%; box-shadow: #AAF 0px 0px 10px; }
    </style>
  </head>

  <body>
    <header>
      <h1>{{.Title}}</h1>
      <p>Generated from <span class="filepath">{{.Filepath}}</span> on <span class="generated">{{.Now}}</span></p>
      <div class="description">
        {{.Description}}
      </div>
    </header>
    <article>
      <div class="flashcards">
{{range .Cards}}
        <div class="flashcard">
          <style type="text/css">
            {{index . 0}}
          </style>
          <div class="front card">
            {{index . 1}}
          </div>
          <div class="delim">⇒</div>
          <div class="back card">
            {{index . 2}}
          </div>
          <div style="clear:both"></div>
        </div>
{{end}}
      </div>
    </article>
  </body>
</html>
`

// Configuration defines application configuration parameters
type Configuration struct {
	Input       string
	Output      string
	Title       string
	Description string
}

// DBData will store data retrieved from the database temporarily
type DBData struct {
	Title       string
	Filepath    string
	Now         string
	Description string
	Cards       [][3]string
}

func makeQueries(dbFile string, data *DBData, conf *Configuration) error {
	db, err := sqlx.Open("sqlite3", dbFile)
	if err != nil {
		return err
	}
	defer db.Close()

	// retrieve
	cols := []Collection{}
	db.Select(&cols, "SELECT * FROM col")

	notes := []Note{}
	db.Select(&notes, "SELECT * FROM notes")

	cards := []Card{}
	db.Select(&cards, "SELECT * FROM cards")

	// check
	if len(cols) != 1 {
		return fmt.Errorf("Expected exactly 1 defined collection in database, got %d", len(cols))
	}
	if len(cards) == 0 {
		return fmt.Errorf("Did not find any cards in database - will not create an empty file")
	}

	// parse JSON collection data
	var models map[string]map[string]interface{}
	err = json.Unmarshal([]byte(cols[0].Models), &models)
	if err != nil {
		return err
	}

	var decks map[string]map[string]interface{}
	err = json.Unmarshal([]byte(cols[0].Decks), &decks)
	if err != nil {
		return err
	}

	// read
	if conf.Title != "" {
		data.Title = conf.Title
	}
	if conf.Description != "" {
		data.Description = conf.Description
	}
	// TODO: it would be nice to retrieve some proper description

	// parse deck information

	/*
		   My cheatsheet:

		   col.models
			 [mid][flds] = [{'name': 'Country Name', 'ord': 0, ...}, ...]
			 [mid][tmpls] = [{'name': 'Areas', 'qfmt': '...', 'afmt': '...', 'ord': 0, ...}]
			 [mid][css] = '.card{...} ...'

			col.decks
			 [did][name] = 'Countries of the World'

			notes
			 .id
			 .mid
			 .flds

			cards
			 .nid
			 .did
			 .ord refers to tmpls
	*/

	decksInfo := map[int]string{}
	for did, d := range decks {
		didInt, err := strconv.Atoi(did)
		if err != nil {
			return err
		}
		decksInfo[didInt] = d["name"].(string)
	}

	css := map[int]string{}
	for mid, m := range models {
		midInt, err := strconv.Atoi(mid)
		if err != nil {
			return err
		}
		css[midInt] = m["css"].(string)
	}

	fieldReplacements := map[int]map[string]int{} // map[mid][fieldname] = ord
	for mid, m := range models {
		midInt, err := strconv.Atoi(mid)
		if err != nil {
			return err
		}
		if fieldReplacements[midInt] == nil {
			fieldReplacements[midInt] = make(map[string]int)
		}
		for _, f := range m["flds"].([]interface{}) {
			fTyped := f.(map[string]interface{})
			ord := fTyped["ord"].(float64)
			fieldname := fTyped["name"].(string)
			fieldReplacements[midInt][fieldname] = int(ord)
		}
	}

	templates := map[int]map[int][2]string{} // map[mid][ord] = (front, back)
	for mid, m := range models {
		midInt, err := strconv.Atoi(mid)
		if err != nil {
			return err
		}
		if templates[midInt] == nil {
			templates[midInt] = make(map[int][2]string)
		}
		for _, t := range m["tmpls"].([]interface{}) {
			tTyped := t.(map[string]interface{})
			qfmt := tTyped["qfmt"].(string)
			afmt := tTyped["afmt"].(string)
			ord := tTyped["ord"].(float64)
			templates[midInt][int(ord)] = [2]string{qfmt, afmt}
		}
	}

	nid2mid := map[int]int{}
	nid2flds := map[int]string{}
	for _, n := range notes {
		nid2mid[n.Id] = n.Mid
		nid2flds[n.Id] = n.Flds
	}

	input := `<input type='text' placeholder='solution' class='type' />`
	deckId := -1
	for _, c := range cards {
		mid := nid2mid[c.Nid]
		fields := strings.Split(nid2flds[c.Nid], "\x1f")
		fmt := templates[mid][c.Ord]

		for fieldname, index := range fieldReplacements[mid] {
			fmt[0] = strings.Replace(fmt[0], "{{"+fieldname+"}}", fields[index], -1)
			fmt[1] = strings.Replace(fmt[1], "{{"+fieldname+"}}", fields[index], -1)
			fmt[0] = strings.Replace(fmt[0], "{{type:"+fieldname+"}}", input, -1)
			fmt[1] = strings.Replace(fmt[1], "{{type:"+fieldname+"}}", input, -1)
			fmt[1] = strings.Replace(fmt[1], "{{FrontSide}}", fmt[0], -1)
		}

		if deckId != -1 && deckId != c.Did && data.Title == "" {
			return errors.New("There are multiple decks in use. So please set the title explicitly using the command line argument")
		}
		deckId = c.Did
		data.Cards = append(data.Cards, [3]string{css[mid], fmt[0], fmt[1]})
	}

	data.Title = decksInfo[deckId]
	return nil
}

func readMediaFile(mediaFile string, mediaData map[string]string) error {
	fd, err := os.Open(mediaFile)
	if err != nil {
		return err
	}

	content, err := ioutil.ReadAll(fd)
	if err != nil {

	}

	err = json.Unmarshal(content, &mediaData)
	if err != nil {
		return err
	}

	return nil
}

// via http://stackoverflow.com/a/24792688
func extractArchive(src, metaDest, mediaDest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := r.Close(); err != nil {
			panic(err)
		}
	}()

	// Closure to address file descriptors issue with all the deferred .Close() methods
	extractAndWriteFile := func(f *zip.File) error {
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer func() {
			if err := rc.Close(); err != nil {
				panic(err)
			}
		}()

		if f.FileInfo().IsDir() {
			os.MkdirAll(filepath.Join(metaDest, f.Name), f.Mode())
		} else {
			path := filepath.Join(mediaDest, f.Name)
			if f.Name == "media" || f.Name == "collection.anki2" {
				path = filepath.Join(metaDest, f.Name)
			}

			os.MkdirAll(filepath.Dir(path), f.Mode())
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer func() {
				if err := f.Close(); err != nil {
					panic(err)
				}
			}()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
		return nil
	}

	for _, f := range r.File {
		err := extractAndWriteFile(f)
		if err != nil {
			return err
		}
	}

	return nil
}

func readDatabase(data *DBData, conf Configuration) error {
	// create temporary directory, extract all data inside
	tempDir, err := ioutil.TempDir("", "anki2html")
	if err != nil {
		return err
	}

	// clean up
	defer os.RemoveAll(tempDir)

	// extract files to temporary directory or target directory
	os.MkdirAll(conf.Output, 0700)
	err = extractArchive(conf.Input, tempDir, conf.Output)
	if err != nil {
		return err
	}

	// rename media files to original name
	media := make(map[string]string)
	err = readMediaFile(filepath.Join(tempDir, "media"), media)
	if err != nil {
		return err
	}
	for filename, original := range media {
		from := filepath.Join(conf.Output, filename)
		to := filepath.Join(conf.Output, original)
		if (len(to) > 0 && to[0] == '/') || (len(to) > 3 && to[0:3] == "../") {
			return errors.New("zip archive contains malicious file path for media file - aborting for security reasons")
		}
		err = os.Rename(from, to)
		if err != nil {
			return err
		}
	}

	// simple values
	data.Filepath = conf.Input
	data.Now = time.Now().Format("2006/01/02")

	// read DB with queries
	err = makeQueries(filepath.Join(tempDir, "collection.anki2"), data, &conf)
	if err != nil {
		return err
	}

	// TODO render flashcards to HTML

	return nil
}

func generateHTMLPage(conf Configuration) error {
	var data DBData

	// read database information
	err := readDatabase(&data, conf)
	if err != nil {
		return err
	}

	// apply HTMLTemplate
	t, err := template.New("anki2html").Parse(HTMLTemplate)
	if err != nil {
		return err
	}

	fd, err := os.Create(filepath.Join(conf.Output, "index.html"))
	if err != nil {
		return err
	}
	defer fd.Close()
	return t.Execute(fd, data)
}

func printHelp() {
	fmt.Println("usage: ./anki2html <file.apkg> [-o <out>] [-t <title>] [-d <description>]")
	fmt.Println("  Takes one APKG file and parses it to a single HTML page.")
	fmt.Println("  The package title can be overwritten with by -t.")
	fmt.Println("  The package description can be overwritten by -d.")
	fmt.Println("  Output written to a folder 'out' or as provided in -o argument.")
}

func main() {
	var conf Configuration

	// argument parser
	var flag string
	for _, a := range os.Args[1:] {
		if len(a) > 0 && a[0] == '-' {
			flag = a[1:]
		} else if flag == "o" || flag == "-output" {
			conf.Output = a
		} else if flag == "t" || flag == "-title" {
			conf.Title = a
		} else if flag == "d" || flag == "-description" {
			conf.Description = a
		} else if flag == "h" || flag == "-help" {
			printHelp()
			os.Exit(0)
		} else {
			if conf.Input != "" {
				printHelp()
				os.Exit(1)
			}
			conf.Input = a
		}
	}

	// default parameters
	if conf.Output == "" {
		conf.Output = "out"
	}

	err := generateHTMLPage(conf)
	if err != nil {
		panic(err)
	}
}
