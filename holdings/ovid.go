package holdings

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	day   = 24 * time.Hour
	month = 30 * day
	year  = 12 * month
)

// delayPattern is how moving walls are expressed in OVID format.
var delayPattern = regexp.MustCompile(`^(-\d+)(M|Y)$`)

var (
	errUnknownUnit   = errors.New("unknown unit")
	errUnknownFormat = errors.New("unknown format")
	errDelayMismatch = errors.New("delay mismatch")
)

// ISSNPattern is the canonical form of an ISSN.
var ISSNPattern = regexp.MustCompile(`^\d\d\d\d-\d\d\d\d$`)

// Holding contains a single holding
type Holding struct {
	EZBID        int           `xml:"ezb_id,attr" json:"ezbid"`
	Title        string        `xml:"title" json:"title"`
	Publishers   string        `xml:"publishers" json:"publishers"`
	PISSN        []string      `xml:"EZBIssns>p-issn" json:"pissn"`
	EISSN        []string      `xml:"EZBIssns>e-issn" json:"eissn"`
	Entitlements []Entitlement `xml:"entitlements>entitlement" json:"entitlements"`
}

// Entitlement holds a single OVID entitlement.
type Entitlement struct {
	Status     string `xml:"status,attr" json:"status"`
	URL        string `xml:"url" json:"url"`
	Anchor     string `xml:"anchor" json:"anchor"`
	FromYear   string `xml:"begin>year" json:"from-year"`
	FromVolume string `xml:"begin>volume" json:"from-volume"`
	FromIssue  string `xml:"begin>issue" json:"from-issue"`
	FromDelay  string `xml:"begin>delay" json:"from-delay"`
	ToYear     string `xml:"end>year" json:"to-year"`
	ToVolume   string `xml:"end>volume" json:"to-volume"`
	ToIssue    string `xml:"end>issue" json:"to-issue"`
	ToDelay    string `xml:"end>delay" json:"to-delay"`
}

// slimmer processing ----

const (
	// LowDatum represents a lowest datum for unspecified start dates
	LowDatum = "0000000000000000"
	// HighDatum represents a lowest datum for unspecified end dates
	HighDatum = "ZZZZZZZZZZZZZZZZ"
)

// CombineDatum combines year, volume and issue into a single value,
// that preserves the order, if length of year, volume and issue do not
// exceed 4, 6 and 6, respectively.
func CombineDatum(year, volume, issue string, empty string) string {
	if year == "" && volume == "" && issue == "" && empty != "" {
		return empty
	}
	return fmt.Sprintf("%04s%06s%06s", year, volume, issue)
}

// Licenses holds the license ranges for an ISSN.
type Licenses map[string][]string

// Add adds a license range string to a given ISSN. Dups are ignored.
func (t Licenses) Add(issn, license string) {
	for _, v := range t[issn] {
		if v == license {
			return
		}
	}
	t[issn] = append(t[issn], license)
}

// slimmer processing ----

// IssnHolding maps an ISSN to a holdings.Holding struct.
// ISSN -> Holding -> []Entitlements
type IssnHolding map[string]Holding

// IsilIssnHolding maps an ISIL to an IssnHolding map.
// ISIL -> ISSN -> Holding -> []Entitlements
type IsilIssnHolding map[string]IssnHolding

// Isils returns available ISILs in this IsilIssnHolding map.
func (iih *IsilIssnHolding) Isils() (keys []string) {
	for k := range *iih {
		keys = append(keys, k)
	}
	return keys
}

// ParseDelay parses delay strings like '-1M', '-3Y', ... into a time.Duration.
func ParseDelay(s string) (d time.Duration, err error) {
	ms := delayPattern.FindStringSubmatch(s)
	if len(ms) != 3 {
		return d, errUnknownFormat
	}
	value, err := strconv.Atoi(ms[1])
	if err != nil {
		return d, err
	}
	switch {
	case ms[2] == "Y":
		d = time.Duration(time.Duration(value) * year)
	case ms[2] == "M":
		d = time.Duration(time.Duration(value) * month)
	default:
		return d, errUnknownUnit
	}
	return
}

// Delay returns the specified delay as `time.Duration`
func (e *Entitlement) Delay() (d time.Duration, err error) {
	if e.FromDelay != "" && e.ToDelay != "" && e.FromDelay != e.ToDelay {
		return d, errDelayMismatch
	}
	if e.FromDelay != "" {
		return ParseDelay(e.FromDelay)
	}
	if e.ToDelay != "" {
		return ParseDelay(e.ToDelay)
	}
	return
}

// Boundary returns the last date before the moving wall restriction becomes effective.
func (e *Entitlement) Boundary() (d time.Time, err error) {
	delay, err := e.Delay()
	if err != nil {
		return d, err
	}
	return time.Now().Add(delay), nil
}

// HoldingsMap creates an ISSN[Holding] struct from a reader.
func HoldingsMap(reader io.Reader) IssnHolding {
	h := make(map[string]Holding)
	decoder := xml.NewDecoder(reader)
	var tag string
	for {
		t, _ := decoder.Token()
		if t == nil {
			break
		}
		switch se := t.(type) {
		case xml.StartElement:
			tag = se.Name.Local
			if tag == "holding" {
				var item Holding
				decoder.DecodeElement(&item, &se)
				for _, id := range item.EISSN {
					tid := strings.TrimSpace(id)
					if ISSNPattern.MatchString(tid) {
						h[tid] = item
					}
				}
				for _, id := range item.PISSN {
					tid := strings.TrimSpace(id)
					if ISSNPattern.MatchString(tid) {
						h[tid] = item
					}
				}
			}
		}
	}
	return h
}
