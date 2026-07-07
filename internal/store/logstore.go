package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Schedule ids used in the zonelog "schedule" column.
const (
	LogScheduleManual = -1
	LogScheduleQuick  = 99
)

type Grouping string

const (
	GroupNone  Grouping = "none"
	GroupHour  Grouping = "hour"
	GroupDay   Grouping = "day" // buckets by weekday (0 = Sunday), like the original
	GroupMonth Grouping = "month"
)

type LogEntry struct {
	Start      int64 `json:"start"` // unix seconds
	ZoneID     int   `json:"zoneId"`
	Seconds    int   `json:"seconds"`
	ScheduleID int   `json:"scheduleId"` // -1 manual, 99 quick run
	Seasonal   int   `json:"seasonal"`   // percent, -1 unknown
	Weather    int   `json:"weather"`    // percent, -1 unknown
}

type Bucket struct {
	Bucket  int `json:"bucket"`
	Seconds int `json:"seconds"`
}

type ZoneSeries struct {
	ZoneID  int      `json:"zoneId"`
	Buckets []Bucket `json:"buckets"`
}

type LogStore struct {
	db *sql.DB
}

func OpenLog(path string) (*LogStore, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite serializes best over a single connection.
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS zonelog(
		date INTEGER NOT NULL,
		zone INTEGER NOT NULL,
		duration INTEGER NOT NULL,
		schedule INTEGER NOT NULL,
		seasonal INTEGER NOT NULL,
		weather INTEGER NOT NULL);
	CREATE INDEX IF NOT EXISTS idx_zonelog_date ON zonelog(date);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("init zonelog schema: %w", err)
	}
	return &LogStore{db: db}, nil
}

func (l *LogStore) Close() error { return l.db.Close() }

// Prune deletes entries older than the retention window and compacts the
// database. retentionMonths <= 0 keeps everything.
func (l *LogStore) Prune(retentionMonths int) (int64, error) {
	if retentionMonths <= 0 {
		return 0, nil
	}
	cutoff := time.Now().AddDate(0, -retentionMonths, 0).Unix()
	res, err := l.db.Exec(`DELETE FROM zonelog WHERE date < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		if _, err := l.db.Exec(`VACUUM`); err != nil {
			return n, fmt.Errorf("vacuum after prune: %w", err)
		}
	}
	return n, nil
}

// LogZoneEvent records one completed zone run.
func (l *LogStore) LogZoneEvent(start time.Time, zoneID int, d time.Duration, scheduleID, seasonal, weather int) error {
	_, err := l.db.Exec(`INSERT INTO zonelog VALUES(?,?,?,?,?,?)`,
		start.Unix(), zoneID, int(d.Seconds()), scheduleID, seasonal, weather)
	return err
}

// Entries returns raw log rows in [start, end], newest first.
func (l *LogStore) Entries(start, end time.Time) ([]LogEntry, error) {
	rows, err := l.db.Query(`SELECT date, zone, duration, schedule, seasonal, weather
		FROM zonelog WHERE date >= ? AND date <= ? ORDER BY date DESC`, start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []LogEntry{}
	for rows.Next() {
		var e LogEntry
		if err := rows.Scan(&e.Start, &e.ZoneID, &e.Seconds, &e.ScheduleID, &e.Seasonal, &e.Weather); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// Grouped sums watering seconds per zone into hour / weekday / month buckets,
// matching the original's GraphZone groupings. Bucketing uses local time.
func (l *LogStore) Grouped(start, end time.Time, g Grouping) ([]ZoneSeries, error) {
	var format string
	switch g {
	case GroupHour:
		format = "%H"
	case GroupDay:
		format = "%w"
	case GroupMonth:
		format = "%m"
	default:
		return nil, fmt.Errorf("unsupported grouping %q", g)
	}
	rows, err := l.db.Query(fmt.Sprintf(`SELECT zone,
			CAST(strftime('%s', date, 'unixepoch', 'localtime') AS INTEGER) AS bucket,
			SUM(duration)
		FROM zonelog WHERE date >= ? AND date <= ?
		GROUP BY zone, bucket ORDER BY zone, bucket`, format), start.Unix(), end.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	series := []ZoneSeries{}
	for rows.Next() {
		var zone, bucket, seconds int
		if err := rows.Scan(&zone, &bucket, &seconds); err != nil {
			return nil, err
		}
		if len(series) == 0 || series[len(series)-1].ZoneID != zone {
			series = append(series, ZoneSeries{ZoneID: zone, Buckets: []Bucket{}})
		}
		s := &series[len(series)-1]
		s.Buckets = append(s.Buckets, Bucket{Bucket: bucket, Seconds: seconds})
	}
	return series, rows.Err()
}
