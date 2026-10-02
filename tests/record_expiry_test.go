package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zadykian/cld/tests/internal/sandbox"
)

// The indexes join gives, and what in the record expires after 30 days (decision 40.6).

// The entry of a session whose server runs stays at any age, with its environment and marks.
// claude's hooks touch it again, and it shows as ended once the session has. An entry that old
// without its server goes, with the files beside it, as do the files of an entry gone.
func TestRecordWhileRunning(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	startCld(t, s, "tmux", nil, "join", "-s", "a")
	a := s.WaitProbes(1)[0]
	waitClients(t, s, 1)
	checkMarks(t, s, "after join", map[string]bool{"a": true})
	writeRestorable(t, s, "c", s.Work, firstID, filepath.Join(sandbox.ProbeBin, "claude"), nil)
	s.WriteFile(companionFile(s, "c", ".busy"), "")
	// o's busy mark is one a hook made as its entry was forgotten.
	s.WriteFile(companionFile(s, "o", ".busy"), "")
	month := 31 * 24 * time.Hour
	var old []string
	for _, name := range []string{"a", "c"} {
		for _, ext := range []string{".json", ".env", ".run", ".busy"} {
			if exists(companionFile(s, name, ext)) {
				old = append(old, companionFile(s, name, ext))
			}
		}
	}
	age(t, month, old...)
	startCld(t, s, "tmux", nil, "join", "-s", "b")
	s.WaitProbes(2)
	waitClients(t, s, 2)
	checkExpiredWhileRunning(t, s)
	a.Hook("Stop", `{"session_id":"`+firstID+`"}`)
	touched(t, s, "a", "Stop")
	killAndWait(t, s, s.Work, a, "-s", "a")
	want := "NAME  STATE     LAST ACTIVE  DIRECTORY\n" +
		"a     ended     -            " + s.Work + "\n" +
		"b     attached  now          " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Code != 0 || result.Stdout != want {
		t.Errorf("list: exit %d, stderr %q, stdout\n%s\nwant\n%s",
			result.Code, result.Stderr, result.Stdout, want)
	}
}

// checkExpiredWhileRunning checks the record in s once join has expired it. Session a, whose
// server runs, keeps its entry and the files beside it, and c and o have none left.
func checkExpiredWhileRunning(t *testing.T, s *sandbox.Sandbox) {
	t.Helper()
	if readEntry(s, "a") != entry("a", s.Work, "") {
		t.Errorf("a's entry %q after another join, want it kept", readEntry(s, "a"))
	}
	for _, ext := range []string{".env", ".run"} {
		if !exists(companionFile(s, "a", ext)) {
			t.Errorf("a%s removed with a's server running, want it kept", ext)
		}
	}
	for _, file := range []string{
		entryFile(s, "c"), companionFile(s, "c", ".env"), companionFile(s, "c", ".run"),
		companionFile(s, "c", ".busy"), companionFile(s, "o", ".busy"),
	} {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("%s: %v, want it removed", filepath.Base(file), err)
		}
	}
}

// joinNamed runs join with args on a terminal, against the fake tmux, and checks that the title
// it prints names session cld-want.
func joinNamed(t *testing.T, s *sandbox.Sandbox, want string, args ...string) {
	t.Helper()
	result := s.RunCldOnTerminal(fakeTmux(s), append([]string{"join"}, args...)...)
	title := "\x1b]0;✳ cld-" + want + "\a"
	if result.Code != 0 || result.Stdout != title || result.Stderr != "" {
		t.Errorf("join %q: exit %d, stdout %q, stderr %q, want exit 0, stdout %q",
			args, result.Code, result.Stdout, result.Stderr, title)
	}
}

// Without -s, join gives the index above those of the sessions that run, of the record's entries
// and of the indexes it gave. So a forgotten entry's index is not given again, and one given with
// -s counts too. The fake tmux says that no server runs.
func TestRecordNames(t *testing.T) {
	t.Parallel()
	s := sandbox.New(t)
	joinNamed(t, s, "0")
	joinNamed(t, s, "1")
	forget(t, s, "1")
	joinNamed(t, s, "2")
	joinNamed(t, s, "7", "-s", "7")
	joinNamed(t, s, "8")
	joinNamed(t, s, "api-0", "-n", "api")
	joinNamed(t, s, "API-1", "-n", "API")
	indexes := filepath.Join(filepath.Dir(filepath.Dir(entryFile(s, "0"))), "indexes.json")
	var given map[string]struct {
		Index int       `json:"index"`
		Time  time.Time `json:"time"`
	}
	if data, err := os.ReadFile(indexes); err != nil || json.Unmarshal(data, &given) != nil {
		t.Fatalf("indexes.json: %v\n%s", err, data)
	}
	if given[""].Index != 8 || given["api-"].Index != 0 || given["API-"].Index != 1 {
		t.Errorf("indexes given %+v, want 8 for none, 0 for api- and 1 for API-", given)
	}
	checkRecordExpires(t, s, indexes)
}

// checkRecordExpires ages TestRecordNames' entries 0, 2, 7 and 8, and the indexes given, by 31
// days: they count no more, and join removes them.
func checkRecordExpires(t *testing.T, s *sandbox.Sandbox, indexes string) {
	t.Helper()
	var old []string
	for _, name := range []string{"0", "2", "7", "8"} {
		old = append(old, entryFile(s, name))
	}
	age(t, 31*24*time.Hour, old...)
	data, err := os.ReadFile(indexes)
	if err != nil {
		t.Fatal(err)
	}
	s.WriteFile(indexes, strings.ReplaceAll(string(data), time.Now().UTC().Format("2006-01-02"),
		time.Now().UTC().AddDate(0, 0, -31).Format("2006-01-02")))
	want := "NAME   STATE     LAST ACTIVE  DIRECTORY\n" +
		"API-1  ended     -            " + s.Work + "\n" +
		"api-0  ended     -            " + s.Work + "\n"
	if result := s.RunCld(nil, "list"); result.Stdout != want {
		t.Errorf("list with the entries expired:\n%s", result.Stdout)
	}
	joinNamed(t, s, "0")
	for _, name := range []string{"2", "7", "8"} {
		if _, err := os.Stat(entryFile(s, name)); !os.IsNotExist(err) {
			t.Errorf("entry %s: %v, want it removed", name, err)
		}
	}
}
