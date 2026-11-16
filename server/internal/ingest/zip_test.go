package ingest

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenXMLRejectsTraversalAndExtraEntries(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.zip")
	writeZip(t, good, map[string]string{
		"export.xml":             `<HealthData></HealthData>`,
		"workout-routes/run.gpx": `<gpx></gpx>`,
	})
	rc, err := OpenXML(good, Limits{MaxEntries: 10, MaxBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()

	bad := filepath.Join(dir, "bad.zip")
	writeZip(t, bad, map[string]string{
		"../export.xml": `<HealthData></HealthData>`,
	})
	if _, err := OpenXML(bad, Limits{MaxEntries: 10, MaxBytes: 1 << 20}); err == nil {
		t.Fatal("expected zip_unsafe")
	}

	huge := filepath.Join(dir, "huge.zip")
	writeZip(t, huge, map[string]string{"export.xml": `<HealthData></HealthData>`})
	if _, err := OpenXML(huge, Limits{MaxEntries: 10, MaxBytes: 4}); err == nil {
		t.Fatal("expected size rejection")
	}
}

func writeZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
