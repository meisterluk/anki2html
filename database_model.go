package main

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"time"
)

// SecondsTime (epoch, int64) is a db-compatible representation of time.Time
type SecondsTime time.Time

// Scan implements the database/sql.Scanner interface
func (s *SecondsTime) Scan(val interface{}) error {
	var secs int64
	var err error
	switch t := val.(type) {
	case int64:
		secs = val.(int64)
	case float64:
		secs = int64(val.(float64))
	case string:
		var seconds int
		seconds, err = strconv.Atoi(val.(string))
		if err != nil {
			return err
		}
		secs = int64(seconds)
	case time.Time:
		secs = time.Time(val.(time.Time)).Unix()
	case nil:
		secs = 0
	default:
		return fmt.Errorf("Cannot convert %s to SecondsTime value", t)
	}

	*s = SecondsTime(time.Unix(secs, 0).UTC())
	return nil
}

// Value implements the database/sql/driver.Value interface
func (s SecondsTime) Value() (driver.Value, error) {
	return time.Time(s).Unix(), nil
}

// MilliSecondsTime (epoch, int64) is a db-compatible representation of time.Time
type MilliSecondsTime time.Time

// Scan implements the database/sql.Scanner interface
func (s *MilliSecondsTime) Scan(val interface{}) error {
	var msecs int64
	var err error
	switch t := val.(type) {
	case int64:
		msecs = val.(int64)
	case float64:
		msecs = int64(val.(float64))
	case string:
		var milliseconds int
		milliseconds, err = strconv.Atoi(val.(string))
		if err != nil {
			return err
		}
		msecs = int64(milliseconds)
	case time.Time:
		msecs = time.Time(val.(time.Time)).UnixNano()
	case nil:
		msecs = 0
	default:
		return fmt.Errorf("Cannot convert %s to MilliSecondsTime value", t)
	}

	*s = MilliSecondsTime(time.Unix(msecs/1000, msecs%1000).UTC())
	return nil
}

// Value implements the database/sql/driver.Value interface
func (s MilliSecondsTime) Value() (driver.Value, error) {
	return time.Time(s).UnixNano(), nil
}

// Apkg represents all data stored in a APKG zip archive
type Apkg struct {
	Cards  []Card
	Col    []Collection
	Graves []Grave
	Notes  []Note
	RevLog RevisionLog
	Media  []Media
}

// A card with associated metadata
// SQL table name: cards
type Card struct {
	Id     int64       `db:"id"`     // id integer primary key, creation timestamp, milliseconds since 1970/1/1
	Nid    int         `db:"nid"`    // nid integer not null, note ID containing card content, notes.Id
	Did    int         `db:"did"`    // did integer not null, deck ID of deck to use, deck ID of deck defined in Collection.Models
	Ord    int         `db:"ord"`    // ord integer not null, template ID of used models to use, template ID of template defined in Collection.Models
	Mod    SecondsTime `db:"mod"`    // mod integer not null, last modification timestamp, seconds since 1970/1/1
	Usn    int         `db:"usn"`    // usn integer not null, update sequence number / synchronization incrementor, -1 or higher
	Typ    int         `db:"type"`   // type integer not null, card type, one of {new, learning, due}
	Queue  int         `db:"queue"`  // queue integer not null, queue, one of {suspended, user buried, sched buried}
	Due    int         `db:"due"`    // due integer not null, count of waiting reviews and cards currently in learning, 0 or more
	Ivl    int         `db:"ivl"`    // ivl integer not null, SRS algorithm interval parameter, ??
	Factor int         `db:"factor"` // factor integer not null, SRS algorithm factor parameter, ??
	Reps   int         `db:"reps"`   // reps integer not null, counter for reviews, 0 or higher
	Lapses int         `db:"lapses"` // lapses integer not null, number of state changes between correct/wrong answer, 0 or higher
	Left   int         `db:"left"`   // left integer not null, ??, ??
	Odue   int         `db:"odue"`   // odue integer not null, 0, 0
	Odid   int         `db:"odid"`   // odid integer not null, 0, 0
	Flags  int         `db:"flags"`  // flags integer not null, 0, 0
	Data   string      `db:"data"`   // data text not null, '', ''
}

// Represents an Anki collection
// SQL table name: col
type Collection struct {
	Id     int64            `db:"id"`     // id integer primary key, collection id, 1 or higher
	Crt    MilliSecondsTime `db:"crt"`    // crt integer not null, creation timestamp, seconds since 1970/1/1
	Mod    MilliSecondsTime `db:"mod"`    // mod integer not null, last modification timestamp, milliseconds since 1970/1/1
	Scm    MilliSecondsTime `db:"scm"`    // scm integer not null, schema modification timestamp, milliseconds since 1970/1/1
	Ver    int              `db:"ver"`    // ver integer not null, API version, currently 11
	Dty    int              `db:"dty"`    // dty integer not null, dirty, always 0
	Usn    int              `db:"usn"`    // usn integer not null, update sequence number / synchronization incrementor, -1 or higher
	Ls     int              `db:"ls"`     // ls integer not null, last synchronization time, milliseconds since 1970/1/1
	Conf   string           `db:"conf"`   // conf text not null, configuration, JSON
	Models string           `db:"models"` // models text not null, model alias note type configuration, JSON
	Decks  string           `db:"decks"`  // decks text not null, decks, JSON
	Dconf  string           `db:"dconf"`  // dconf text not null, deck configuration, JSON
	Tags   string           `db:"tags"`   // tags text not null, tags, ??
}

// ??
// SQL table name: graves
type Grave struct {
	Usn  int `db:"usn"`  // usn integer not null, update sequence number / synchronization incrementor, -1 or higher
	Oid  int `db:"oid"`  // oid integer not null, ??, ??
	Type int `db:"type"` // type integer not null, ??, ??
}

// Note providing additional/sharable data/information for cards
// SQL table name: notes
type Note struct {
	Id    int    `db:"id"`    // id integer primary key, creation timestamp, seconds since 1970/1/1
	Guid  string `db:"guid"`  // guid text not null, global ID, random 10-character string?!
	Mid   int    `db:"mid"`   // mid integer not null, model ID used,
	Mod   int    `db:"mod"`   // mod integer not null, modified timestamp, seconds since 1970/1/1
	Usn   int    `db:"usn"`   // usn integer not null, update sequence number / synchronization incrementor, -1 or higher
	Tags  string `db:"tags"`  // tags text not null, unprocessed tags, string
	Flds  string `db:"flds"`  // flds text not null, values of fields separated by \x1F, string
	Sfld  string `db:"sfld"`  // sfld integer not null, text of the first value, string
	Csum  int    `db:"csum"`  // csum integer not null, duplicate check field checksum, first field's first 8 digit's SHA1 sum's integer representation
	Flags int    `db:"flags"` // flags integer not null, 0, unused
	Data  string `db:"data"`  // data text not null, '', unused
}

// Review log logging all reviews done by the user
// SQL table name: revlog
type RevisionLog struct {
	Id      int   `db:"id"`      // id integer primary key, time of review, seconds since 1970/1/1
	Cid     int64 `db:"cid"`     // cid integer not null, which card was reviewed, card.Id
	Usn     int   `db:"usn"`     // usn integer not null, update sequence number / synchronization incrementor, -1 or higher
	Ease    int   `db:"ease"`    // ease integer not null, rating given by user, one of {wrong, hard, ok, easy}
	Ivl     int   `db:"ivl"`     // ivl integer not null, SRS interval, seconds
	LastIvl int   `db:"lastIvl"` // lastIvl integer not null, previous SRS interval, seconds
	Factor  int   `db:"factor"`  // factor integer not null, SRS factor, 1 or higher ??
	Time    int   `db:"time"`    // time integer not null, review duration, seconds most likely smaller than 60
	Type    int   `db:"type"`    // type integer not null, review type, one of {learn, review, relearn, cram}
}

type Media struct {
	Filepath string
}
