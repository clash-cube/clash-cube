package profiles

import (
	"os"
	"sync"
	"testing"

	"github.com/localhost-copilot/clashcube/internal/appdir"
)

func TestContentEditsPreserveSourceAndRejectStaleOrManagedWrites(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if err := appdir.Ensure(); err != nil {
		t.Fatal(err)
	}
	const original = "# keep comments\nrules: [\"MATCH,DIRECT\"]\n"
	p, err := ImportContent("Local", []byte(original))
	if err != nil {
		t.Fatal(err)
	}
	before, err := ReadDocument(p.ID)
	if err != nil || before.Body != original || before.ReadOnly {
		t.Fatalf("read local document: %+v, %v", before, err)
	}
	if _, err = SaveContent(p.ID, before.Revision, "rules: ["); err == nil {
		t.Fatal("saved invalid YAML")
	}
	if current, _ := ReadDocument(p.ID); current != before {
		t.Fatal("failed edit modified the source")
	}
	const changed = "# 手动编辑\nrules: [\"MATCH,REJECT\"]\n"
	after, err := SaveContent(p.ID, before.Revision, changed)
	if err != nil || after.Body != changed || after.Revision == before.Revision {
		t.Fatalf("save local document: %+v, %v", after, err)
	}
	if _, err = SaveContent(p.ID, before.Revision, original); err == nil {
		t.Fatal("stale editor overwrote a newer save")
	}
	fi, err := os.Stat(p.Path())
	if err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("saved credentials are not private: %v", err)
	}
	sub, err := add(Profile{ID: newID(), Name: "Managed", URL: "https://example.com/sub"}, []byte(original))
	if err != nil {
		t.Fatal(err)
	}
	managed, err := ReadDocument(sub.ID)
	if err != nil || !managed.ReadOnly {
		t.Fatal("subscription must be read-only")
	}
	if _, err = SaveContent(sub.ID, managed.Revision, changed); err == nil {
		t.Fatal("edited a managed subscription")
	}
	if _, err = ReadDocument("../../outside"); err == nil {
		t.Fatal("read a caller-supplied path")
	}
}

func TestConcurrentContentSavesCannotLoseAnUpdate(t *testing.T) {
	t.Setenv("CLASHCUBE_HOME", t.TempDir())
	if err := appdir.Ensure(); err != nil {
		t.Fatal(err)
	}
	p, err := ImportContent("Concurrent", []byte("rules: [\"MATCH,DIRECT\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := ReadDocument(p.ID)
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, value := range []string{"# first\n", "# second\n"} {
		workers.Add(1)
		go func(value string) {
			defer workers.Done()
			<-start
			_, err := SaveContent(p.ID, doc.Revision, value+doc.Body)
			results <- err
		}(value)
	}
	close(start)
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("%d competing writes succeeded", successes)
	}
}
