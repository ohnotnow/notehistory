package notehistory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"howett.net/plist"
)

// coreDataEpoch is seconds from 1970-01-01 to 2001-01-01.
const coreDataEpoch = 978307200

// record mirrors the fields we care about inside Notification Centre's plist
// blob. Everything else in the blob is ignored.
type record struct {
	App string `plist:"app"`
	// Date is a plist date in some records and a bare Core Data float in
	// others, so it is decoded loosely and sorted out in when().
	Date any `plist:"date"`
	Req  struct {
		Title    string `plist:"titl"`
		Subtitle string `plist:"subt"`
		Body     string `plist:"body"`
	} `plist:"req"`
}

// CurrentRows reads every notification currently in Notification Centre's
// database. Rows whose plist cannot be decoded are reported on stderr and
// skipped rather than aborting the whole read.
func CurrentRows(ctx context.Context, sourceDB string) ([]Notification, error) {
	db, err := sql.Open("sqlite", "file:"+sourceDB+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `SELECT r.rec_id, r.uuid, a.identifier, r.delivered_date, r.request_date, r.data
		FROM record r LEFT JOIN app a ON a.app_id = r.app_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var recID int64
		var uuid, data []byte
		var app sql.NullString
		var delivered, requested sql.NullFloat64
		if err := rows.Scan(&recID, &uuid, &app, &delivered, &requested, &data); err != nil {
			return nil, err
		}
		var rec record
		if _, err := plist.Unmarshal(data, &rec); err != nil {
			fmt.Fprintf(os.Stderr, "could not decode record %d: %v\n", recID, err)
			continue
		}
		n := Notification{
			UUID:        uuidKey(uuid, recID),
			DeliveredAt: when(delivered, requested, rec.Date),
			App:         app.String,
			Title:       rec.Req.Title,
			Subtitle:    rec.Req.Subtitle,
			Body:        rec.Req.Body,
		}
		if n.App == "" {
			n.App = rec.App
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func uuidKey(uuid []byte, recID int64) string {
	if len(uuid) == 0 {
		return fmt.Sprintf("rec:%d", recID)
	}
	// A 16-byte blob is a raw UUID; anything else is already text.
	if len(uuid) == 16 || bytes.ContainsFunc(uuid, func(r rune) bool { return r < 0x20 || r > 0x7e }) {
		return hex.EncodeToString(uuid)
	}
	return string(uuid)
}

func when(delivered, requested sql.NullFloat64, fallback any) string {
	for _, v := range []sql.NullFloat64{delivered, requested} {
		if v.Valid && v.Float64 != 0 {
			return coreDataTime(v.Float64)
		}
	}
	switch d := fallback.(type) {
	case time.Time:
		return d.Local().Format(timeLayout)
	case float64:
		return coreDataTime(d)
	}
	return ""
}

func coreDataTime(secs float64) string {
	return time.Unix(int64(secs)+coreDataEpoch, 0).Format(timeLayout)
}
