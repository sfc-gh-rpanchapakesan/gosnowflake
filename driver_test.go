package gosnowflake

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"
)

var (
	user       string
	pass       string
	account    string
	dbname     string
	schemaname string
	warehouse  string
	dsn        string
	host       string
	port       string
	protocol   string
	available  bool
)

var (
	tDate      = time.Date(2012, 6, 14, 0, 0, 0, 0, time.UTC)
	sDate      = "2012-06-14"
	tDateTime  = time.Date(2011, 11, 20, 21, 27, 37, 0, time.UTC)
	sDateTime  = "2011-11-20 21:27:37"
	tDate0     = time.Time{}
	sDate0     = "0000-00-00"
	sDateTime0 = "0000-00-00 00:00:00"
)

// TODO: Document how to setup the test parameters
func init() {
	// get environment variables
	env := func(key, defaultValue string) string {
		if value := os.Getenv(key); value != "" {
			return value
		}
		return defaultValue
	}
	user = env("SNOWFLAKE_TEST_USER", "testuser")
	pass = env("SNOWFLAKE_TEST_PASSWORD", "testpassword")
	account = env("SNOWFLAKE_TEST_ACCOUNT", "testaccount")
	dbname = env("SNOWFLAKE_TEST_DATABASE", "testdb")
	schemaname = env("SNOWFLAKE_TEST_SCHEMA", "public")
	warehouse = env("SNOWFLAKE_TEST_WAREHOUSE", "testwarehouse")

	protocol = env("SNOWFLAKE_TEST_PROTOCOL", "https")
	host = os.Getenv("SNOWFLAKE_TEST_HOST")
	port = os.Getenv("SNOWFLAKE_TEST_PORT")
	if host == "" {
		host = fmt.Sprintf("%s.snowflakecomputing.com", account)
	} else {
		host = fmt.Sprintf("%s:%s", host, port)
	}

	dsn = fmt.Sprintf("%s:%s@%s/%s/%s", user, pass, host, dbname, schemaname)

	parameters := url.Values{}
	parameters.Add("timezone", "UTC") // TODO: do we want to support this? This is good for tests.
	if protocol != "" {
		parameters.Add("protocol", protocol)
	}
	if account != "" {
		parameters.Add("account", account)
	}
	if warehouse != "" {
		parameters.Add("warehouse", warehouse)
	}
	if len(parameters) > 0 {
		dsn += "?" + parameters.Encode()
	}
}

type DBTest struct {
	*testing.T
	db *sql.DB
}

func (dbt *DBTest) mustQuery(query string, args ...interface{}) (rows *sql.Rows) {
	rows, err := dbt.db.Query(query, args...)
	if err != nil {
		dbt.fail("query", query, err)
	}
	return rows
}

func (dbt *DBTest) fail(method, query string, err error) {
	if len(query) > 300 {
		query = "[query too large to print]"
	}
	dbt.Fatalf("error on %s [%s]: %s", method, query, err.Error())
}

func (dbt *DBTest) mustExec(query string, args ...interface{}) (res sql.Result) {
	res, err := dbt.db.Exec(query, args...)
	if err != nil {
		dbt.fail("exec", query, err)
	}
	return res
}

func runTests(t *testing.T, dsn string, tests ...func(dbt *DBTest)) {
	db, err := sql.Open("snowflake", dsn)
	if err != nil {
		t.Fatalf("error connecting: %s", err.Error())
	}
	defer db.Close()

	db.Exec("DROP TABLE IF EXISTS test")

	dbt := &DBTest{t, db}
	for _, test := range tests {
		test(dbt)
		dbt.db.Exec("DROP TABLE IF EXISTS test")
	}
}

func TestEmptyQuery(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		query := "--"
		// just a comment, no query
		_, err := dbt.db.Query(query)
		if err == nil {
			dbt.fail("query", query, err)
		}
		if driverErr, ok := err.(*SnowflakeError); ok {
			if driverErr.Number != 900 { // syntax error
				dbt.fail("query", query, err)
			}
		}
	})
}

func TestCRUD(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		// Create Table
		dbt.mustExec("CREATE TABLE test (value BOOLEAN)")

		// Test for unexpected Data
		var out bool
		rows := dbt.mustQuery("SELECT * FROM test")
		if rows.Next() {
			dbt.Error("unexpected Data in empty table")
		}

		// Create Data
		res := dbt.mustExec("INSERT INTO test VALUES (true)")
		count, err := res.RowsAffected()
		if err != nil {
			dbt.Fatalf("res.RowsAffected() returned error: %s", err.Error())
		}
		if count != 1 {
			dbt.Fatalf("expected 1 affected row, got %d", count)
		}

		id, err := res.LastInsertId()
		if err != nil {
			dbt.Fatalf("res.LastInsertId() returned error: %s", err.Error())
		}
		if id != -1 {
			dbt.Fatalf(
				"expected InsertId -1, got %d. Snowflake doesn't support last insert ID", id)
		}

		// Read
		rows = dbt.mustQuery("SELECT value FROM test")
		if rows.Next() {
			rows.Scan(&out)
			if true != out {
				dbt.Errorf("true != %t", out)
			}

			if rows.Next() {
				dbt.Error("unexpected Data")
			}
		} else {
			dbt.Error("no Data")
		}

		// Update
		res = dbt.mustExec("UPDATE test SET value = ? WHERE value = ?", false, true)
		count, err = res.RowsAffected()
		if err != nil {
			dbt.Fatalf("res.RowsAffected() returned error: %s", err.Error())
		}
		if count != 1 {
			dbt.Fatalf("expected 1 affected row, got %d", count)
		}

		// Check Update
		rows = dbt.mustQuery("SELECT value FROM test")
		if rows.Next() {
			rows.Scan(&out)
			if false != out {
				dbt.Errorf("false != %t", out)
			}

			if rows.Next() {
				dbt.Error("unexpected Data")
			}
		} else {
			dbt.Error("no Data")
		}

		// Delete
		res = dbt.mustExec("DELETE FROM test WHERE value = ?", false)
		count, err = res.RowsAffected()
		if err != nil {
			dbt.Fatalf("res.RowsAffected() returned error: %s", err.Error())
		}
		if count != 1 {
			dbt.Fatalf("expected 1 affected row, got %d", count)
		}

		// Check for unexpected rows
		res = dbt.mustExec("DELETE FROM test")
		count, err = res.RowsAffected()
		if err != nil {
			dbt.Fatalf("res.RowsAffected() returned error: %s", err.Error())
		}
		if count != 0 {
			dbt.Fatalf("expected 0 affected row, got %d", count)
		}
	})
}

func TestInt(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		types := []string{"INT", "INTEGER"}
		in := int64(42)
		var out int64
		var rows *sql.Rows

		// SIGNED
		for _, v := range types {
			dbt.mustExec("CREATE TABLE test (value " + v + ")")

			dbt.mustExec("INSERT INTO test VALUES (?)", in)

			rows = dbt.mustQuery("SELECT value FROM test")
			if rows.Next() {
				rows.Scan(&out)
				if in != out {
					dbt.Errorf("%s: %d != %d", v, in, out)
				}
			} else {
				dbt.Errorf("%s: no data", v)
			}

			dbt.mustExec("DROP TABLE IF EXISTS test")
		}
	})
}

func TestFloat32(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		types := [2]string{"FLOAT", "DOUBLE"}
		in := float32(42.23)
		var out float32
		var rows *sql.Rows
		for _, v := range types {
			dbt.mustExec("CREATE TABLE test (value " + v + ")")
			dbt.mustExec("INSERT INTO test VALUES (?)", in)
			rows = dbt.mustQuery("SELECT value FROM test")
			if rows.Next() {
				rows.Scan(&out)
				if in != out {
					dbt.Errorf("%s: %g != %g", v, in, out)
				}
			} else {
				dbt.Errorf("%s: no data", v)
			}
			dbt.mustExec("DROP TABLE IF EXISTS test")
		}
	})
}

func TestFloat64(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		types := [2]string{"FLOAT", "DOUBLE"}
		var expected float64 = 42.23
		var out float64
		var rows *sql.Rows
		for _, v := range types {
			dbt.mustExec("CREATE TABLE test (value " + v + ")")
			dbt.mustExec("INSERT INTO test VALUES (42.23)")
			rows = dbt.mustQuery("SELECT value FROM test")
			if rows.Next() {
				rows.Scan(&out)
				if expected != out {
					dbt.Errorf("%s: %g != %g", v, expected, out)
				}
			} else {
				dbt.Errorf("%s: no data", v)
			}
			dbt.mustExec("DROP TABLE IF EXISTS test")
		}
	})
}

func TestFloat64Placeholder(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		types := [2]string{"FLOAT", "DOUBLE"}
		var expected float64 = 42.23
		var out float64
		var rows *sql.Rows
		for _, v := range types {
			dbt.mustExec("CREATE TABLE test (id int, value " + v + ")")
			dbt.mustExec("INSERT INTO test VALUES (1, 42.23)")
			rows = dbt.mustQuery("SELECT value FROM test WHERE id = ?", 1)
			if rows.Next() {
				rows.Scan(&out)
				if expected != out {
					dbt.Errorf("%s: %g != %g", v, expected, out)
				}
			} else {
				dbt.Errorf("%s: no data", v)
			}
			dbt.mustExec("DROP TABLE IF EXISTS test")
		}
	})
}

func TestString(t *testing.T) {
	runTests(t, dsn, func(dbt *DBTest) {
		types := []string{"CHAR(255)", "VARCHAR(255)", "TEXT", "STRING"}
		in := "κόσμε üöäßñóùéàâÿœ'îë Árvíztűrő いろはにほへとちりぬるを イロハニホヘト דג סקרן чащах  น่าฟังเอย"
		var out string
		var rows *sql.Rows

		for _, v := range types {
			dbt.mustExec("CREATE TABLE test (value " + v + ")")

			dbt.mustExec("INSERT INTO test VALUES (?)", in)

			rows = dbt.mustQuery("SELECT value FROM test")
			if rows.Next() {
				rows.Scan(&out)
				if in != out {
					dbt.Errorf("%s: %s != %s", v, in, out)
				}
			} else {
				dbt.Errorf("%s: no data", v)
			}

			dbt.mustExec("DROP TABLE IF EXISTS test")
		}

		// BLOB (Snowflake doesn't support BLOB type but STRING covers large text data)
		dbt.mustExec("CREATE TABLE test (id int, value STRING)")

		id := 2
		in = "Lorem ipsum dolor sit amet, consetetur sadipscing elitr, " +
		  "sed diam nonumy eirmod tempor invidunt ut labore et dolore magna aliquyam erat, " +
		  "sed diam voluptua. At vero eos et accusam et justo duo dolores et ea rebum. " +
		  "Stet clita kasd gubergren, no sea takimata sanctus est Lorem ipsum dolor sit amet. " +
		  "Lorem ipsum dolor sit amet, consetetur sadipscing elitr, " +
		  "sed diam nonumy eirmod tempor invidunt ut labore et dolore magna aliquyam erat, " +
		  "sed diam voluptua. At vero eos et accusam et justo duo dolores et ea rebum. " +
		  "Stet clita kasd gubergren, no sea takimata sanctus est Lorem ipsum dolor sit amet."
		dbt.mustExec("INSERT INTO test VALUES (?, ?)", id, in)

		err := dbt.db.QueryRow("SELECT value FROM test WHERE id = ?", id).Scan(&out)
		if err != nil {
			dbt.Fatalf("Error on BLOB-Query: %s", err.Error())
		} else if out != in {
			dbt.Errorf("BLOB: %s != %s", in, out)
		}
	})
}

type timeTests struct {
	dbtype  string
	tlayout string
	tests   []timeTest
}

type timeTest struct {
	s string
	t time.Time
}

func (tt timeTest) genQuery() string {
	return "SELECT '%s'::%s"
}

func (tt timeTest) run(t *testing.T, dbt *DBTest, dbtype, tlayout string) {
	var rows *sql.Rows
	query := tt.genQuery()
	query = fmt.Sprintf(query, tt.s, dbtype)
	rows = dbt.mustQuery(query)
	defer rows.Close()
	var err error
	if !rows.Next() {
		err = rows.Err()
		if err == nil {
			err = fmt.Errorf("no data")
		}
		dbt.Errorf("%s: %s", dbtype, err)
		return
	}

	var dst interface{}
	err = rows.Scan(&dst)
	if err != nil {
		dbt.Errorf("%s: %s", dbtype, err)
		return
	}
	switch val := dst.(type) {
	case []uint8:
		str := string(val)
		if str == tt.s {
			return
		}
		dbt.Errorf("%s to string: expected %q, got %q",
			dbtype,
			tt.s,
			str,
		)
	case time.Time:
		if val == tt.t {
			return
		}
		t.Logf("tlayout: %s, v:%s, s:%s, t:%s", tlayout, val, tt.s, tt.t)
		dbt.Errorf("%s to string: expected %q, got %q",
			dbtype,
			tt.s,
			val.Format(tlayout),
		)
	default:
		fmt.Printf("%#v\n", []interface{}{dbtype, tlayout, tt.s, tt.t})
		dbt.Errorf("%s: unhandled type %T (is '%v')",
			dbtype, val, val,
		)
	}
}

func TestDateTime(t *testing.T) {
	afterTime := func(t time.Time, d string) time.Time {
		dur, err := time.ParseDuration(d)
		if err != nil {
			panic(err)
		}
		return t.Add(dur)
	}
	format := "2006-01-02 15:04:05.999999999"
	t0 := time.Time{}
	tstr0 := "0000-00-00 00:00:00.000000000"
	testcases := []timeTests{
		{"DATE", format[:10], []timeTest{
			{t: time.Date(2011, 11, 20, 0, 0, 0, 0, time.UTC)},
			{t: time.Date(2, 8, 2, 0, 0, 0, 0, time.UTC), s: "0002-08-02"},
			// 0000-00-00 is not supported but returns a consistent result
			{t: time.Date(2, 11, 30, 0, 0, 0, 0, time.UTC), s: "0000-00-00"},
		}},
		{"TIME", format[11:19], []timeTest{
			{t: afterTime(t0, "12345s")},
			{t: t0, s: tstr0[11:19]},
		}},
		{"TIME(0)", format[11:19], []timeTest{
			{t: afterTime(t0, "12345s")},
			{t: t0, s: tstr0[11:19]},
		}},
		{"TIME(1)", format[11:21], []timeTest{
			{t: afterTime(t0, "12345600ms")},
			{t: t0, s: tstr0[11:21]},
		}},
		{"TIME(6)", format[11:], []timeTest{
			{t: t0, s: tstr0[11:]},
		}},
		{"DATETIME", format[:19], []timeTest{
			{t: time.Date(2011, 11, 20, 21, 27, 37, 0, time.UTC)},
		}},
		{"DATETIME(0)", format[:21], []timeTest{
			{t: time.Date(2011, 11, 20, 21, 27, 37, 0, time.UTC)},
		}},
		{"DATETIME(1)", format[:21], []timeTest{
			{t: time.Date(2011, 11, 20, 21, 27, 37, 100000000, time.UTC)},
		}},
		{"DATETIME(6)", format, []timeTest{
			{t: time.Date(2011, 11, 20, 21, 27, 37, 123456000, time.UTC)},
		}},
		{"DATETIME(9)", format, []timeTest{
			{t: time.Date(2011, 11, 20, 21, 27, 37, 123456789, time.UTC)},
		}},
	}
	runTests(t, dsn, func(dbt *DBTest) {
		for _, setups := range testcases {
			for _, setup := range setups.tests {
				if setup.s == "" {
					// fill time string wherever Go can reliable produce it
					setup.s = setup.t.Format(setups.tlayout)
				}
				setup.run(t, dbt, setups.dbtype, setups.tlayout)
			}
		}
	})
}
