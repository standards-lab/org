package data

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/standards-lab/sqlate/query"
)

// The keyset cursor. A cursor names the listing that issued it, the sort
// terms it was issued under, and the sort values of the last row of the
// page it continues from, so the next page is the rows after that row in
// the same order. The composer turns it into a WHERE predicate over the
// sort terms: one comparison for a single term, and for several terms the
// expanded form (a > x) OR (a = x AND b > y) OR ..., every value bound
// through the field's declared type as a filter value is.
//
// The encoding is opaque to the caller: base64url over a checksum and a
// JSON body. The checksum is an integrity check, not authentication: an
// edited or truncated cursor is refused with a CursorError before any SQL
// is composed, and so is a cursor issued by the other listing, one issued
// under another sort, and one whose values do not parse as their field's
// type. A cursor carries no secret, and a caller who forges a well-formed
// one gains nothing they could not ask for with a filter.

// CursorError reports a cursor the listing refused before any SQL ran: a
// malformed or edited cursor, one issued by another listing or under
// another sort, or a sort a cursor cannot continue (one that mixes
// directions, or one over a field that can be NULL). It unwraps to
// query.ErrDirectives, the request sentinel.
type CursorError struct {
	Reason string
}

func (e *CursorError) Error() string { return "data: cursor: " + e.Reason }

func (e *CursorError) Unwrap() error { return query.ErrDirectives }

// cursorVersion is the encoding's version, the first field of the body,
// so a cursor from a later encoding is refused rather than misread.
const cursorVersion = 1

// checksumLen is the length of the checksum prefix, in bytes: the first
// bytes of the body's SHA-256.
const checksumLen = 8

// cursor is a decoded cursor.
type cursor struct {
	Version int      `json:"v"`
	Listing string   `json:"listing"`
	Sort    []term   `json:"sort"`
	Values  []string `json:"values"`
}

// term is one sort term of a cursor: a declared field and its direction.
type term struct {
	Field      string `json:"field"`
	Descending bool   `json:"desc,omitempty"`
}

// encode renders c as the opaque string a caller passes back.
func encode(c cursor) (string, error) {
	c.Version = cursorVersion
	body, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("data: encode cursor: %w", err)
	}
	sum := sha256.Sum256(body)
	return base64.RawURLEncoding.EncodeToString(append(sum[:checksumLen], body...)), nil
}

// decode reads an encoded cursor back and checks its integrity: the
// encoding, the checksum, the version, and the shape (one value per
// term). It does not know the listing, so the listing checks the rest.
func decode(s string) (cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) <= checksumLen {
		return cursor{}, &CursorError{Reason: "the cursor is not one this listing issued (malformed)"}
	}
	sum := sha256.Sum256(raw[checksumLen:])
	if string(sum[:checksumLen]) != string(raw[:checksumLen]) {
		return cursor{}, &CursorError{Reason: "the cursor is not one this listing issued (checksum mismatch)"}
	}
	var c cursor
	if err := json.Unmarshal(raw[checksumLen:], &c); err != nil {
		return cursor{}, &CursorError{Reason: "the cursor is not one this listing issued (malformed body)"}
	}
	if c.Version != cursorVersion {
		return cursor{}, &CursorError{Reason: fmt.Sprintf("the cursor is version %d; this listing issues version %d", c.Version, cursorVersion)}
	}
	if len(c.Sort) == 0 || len(c.Sort) != len(c.Values) {
		return cursor{}, &CursorError{Reason: "the cursor's sort terms and values do not match"}
	}
	return c, nil
}

// check confirms c continues this listing under these terms: the listing
// that issued it is this one, the terms are the same fields in the same
// order and directions, and every value parses as its field's declared
// type. The comparison is by terms, so a cursor issued for one directory
// continues the same listing of any directory: the cursor is a position
// in a sort order, not a bookmark on a row.
func (l listing[T]) check(c cursor, terms []term) error {
	if c.Listing != l.plain.Name() {
		return &CursorError{Reason: fmt.Sprintf("the cursor was issued by %s, not by %s", c.Listing, l.plain.Name())}
	}
	if !termsEqual(c.Sort, terms) {
		return &CursorError{Reason: fmt.Sprintf("the cursor was issued under the sort %s; this listing sorts by %s", spell(c.Sort), spell(terms))}
	}
	for i, t := range terms {
		if err := checkValue(l.fields[t.Field], c.Values[i]); err != nil {
			return &CursorError{Reason: fmt.Sprintf("the cursor's value for %s: %v", t.Field, err)}
		}
	}
	return nil
}

// checkValue confirms a cursor value's text parses as the field's declared
// type, for the types the listings declare; any other type is bound as
// text and left to the engine's cast.
func checkValue(f query.Field, v string) error {
	switch f.Type {
	case "uuid":
		if _, err := uuid.Parse(v); err != nil {
			return errors.New("not a uuid")
		}
	case "bigint", "integer", "smallint":
		if _, err := strconv.ParseInt(v, 10, 64); err != nil {
			return errors.New("not an integer")
		}
	case "timestamp with time zone", "timestamp":
		if _, err := time.Parse(time.RFC3339Nano, v); err != nil {
			return errors.New("not a timestamp")
		}
	}
	return nil
}

// sampleValue returns a value of the field's declared type that checkValue
// accepts, for the rendering Verify prepares and never runs.
func sampleValue(f query.Field) string {
	switch f.Type {
	case "uuid":
		return "00000000-0000-0000-0000-000000000000"
	case "bigint", "integer", "smallint":
		return "0"
	case "timestamp with time zone", "timestamp":
		return "1970-01-01T00:00:00Z"
	}
	return ""
}

// termsEqual reports whether two term lists name the same fields in the
// same order with the same directions.
func termsEqual(a, b []term) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// spell renders terms for a message: field, or field desc, comma
// separated.
func spell(terms []term) string {
	parts := make([]string, len(terms))
	for i, t := range terms {
		parts[i] = t.Field
		if t.Descending {
			parts[i] += " desc"
		}
	}
	return strings.Join(parts, ", ")
}

// valuesOf reads the sort values of row for terms, as the text the cursor
// carries: a time at full precision in UTC, an integer in decimal, a
// string as it is, a pointer through its target. A NULL is refused,
// because a cursor cannot position after a NULL; the composer never issues
// a cursor over a field that can be NULL, so a NULL here is a defect.
func valuesOf[T any](row T, terms []term) ([]string, error) {
	fields := fieldsOf(reflect.TypeFor[T]())
	rv := reflect.ValueOf(row)
	out := make([]string, len(terms))
	for i, t := range terms {
		idx, ok := fields[t.Field]
		if !ok {
			return nil, fmt.Errorf("data: cursor: %s has no field %s", rv.Type(), t.Field)
		}
		v := rv.Field(idx)
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return nil, fmt.Errorf("data: cursor: %s is NULL on the last row", t.Field)
			}
			v = v.Elem()
		}
		if ts, ok := v.Interface().(time.Time); ok {
			out[i] = ts.UTC().Format(time.RFC3339Nano)
			continue
		}
		switch v.Kind() {
		case reflect.String:
			out[i] = v.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			out[i] = strconv.FormatInt(v.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			out[i] = strconv.FormatUint(v.Uint(), 10)
		default:
			return nil, fmt.Errorf("data: cursor: %s has a %s value, which a cursor cannot carry", t.Field, v.Type())
		}
	}
	return out, nil
}

// nullableOf indexes the fields of T that scan a NULL: the pointer-typed
// fields, by column name. The key is left out: it is unique, and the
// listing statements never return a row whose key is NULL.
func nullableOf[T any](key string) map[string]bool {
	t := reflect.TypeFor[T]()
	out := map[string]bool{}
	for column, idx := range fieldsOf(t) {
		if column != key && t.Field(idx).Type.Kind() == reflect.Pointer {
			out[column] = true
		}
	}
	return out
}
